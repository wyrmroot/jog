package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/shlex"
)

const (
	STATUS_COL_W  int64 = 40
	LOOP_DURATION       = 1 * time.Second
)

type Status int

var ExEr *exec.ExitError

type QueuedAction struct {
	id      int
	attempt int
	args    []string
}

type TaskResult struct {
	err  error
	code int
}

const (
	Queued Status = iota
	Running
	Completed
	Errored
)

var ColorMap = map[Status]string{
	Queued:    ".",                // Default dot
	Running:   "\033[34m|\033[0m", // Blue line
	Completed: "\033[32m|\033[0m", // Green line
	Errored:   "\033[31m|\033[0m", // Red line
}

type token struct{}

type RunnerConfig struct {
	Attempts      uint
	MaxDelay      uint
	MinDelay      uint
	Concurrency   uint
	sentinel_code int
	AbortOnError  bool // SentinelCode int // Exit code which causes all jobs to abort
	PrintStatus   bool
	Log           bool
	Dir           string
}

type Runner struct {
	sem           chan token
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	count         atomic.Int64
	started       atomic.Int64
	errored       atomic.Int64
	completed     atomic.Int64
	cfg           *RunnerConfig
	attemptDelays []time.Duration
	syslog        io.Writer
	ExitCode      int
}

func NewRunner(cfg *RunnerConfig) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := Runner{
		ctx:           ctx,
		cancel:        cancel,
		wg:            sync.WaitGroup{},
		cfg:           cfg,
		attemptDelays: make([]time.Duration, cfg.Attempts),
	}

	if r.cfg.Log {
		r.syslog = NewSysLogger()
	} else {
		r.syslog = &NullLogger{}
	}

	// Limit concurrency with semaphore
	if r.cfg.Concurrency > 0 {
		slog.Debug("runner will limit number of tasks", "n", cfg.Concurrency)
		r.sem = make(chan token, cfg.Concurrency)
	}

	// Generate retry timings, always starting with 0 for the first
	for i := range r.cfg.Attempts {
		slog.Debug("add delay", "i", i)
		nextWait := r.cfg.MinDelay * uint(math.Pow(2, float64(i-1)))
		r.attemptDelays[i] = time.Duration(min(r.cfg.MaxDelay, nextWait)) * time.Millisecond
	}
	slog.Debug("delay table", "attemptDelays", r.attemptDelays)

	if cfg.PrintStatus {
		go func() {
			for {
				select {
				case <-r.ctx.Done():
					return
				default:
					r.PrintBar()
					time.Sleep(LOOP_DURATION)
				}
			}
		}()
	}

	return &r
}

func (r *Runner) manageTask(task *QueuedAction) {
	defer r.Done()
	var err error
	var exitCode int
	timer := time.NewTimer(0)
	for i, delay := range r.attemptDelays {
		timer.Reset(delay)
		select {
		case <-r.ctx.Done():
			return
		case <-timer.C:
		}

		cmd := exec.CommandContext(r.ctx, task.args[0], task.args[1:]...)
		if r.cfg.Log {
			cmd.Stdout = LogToStdErr(task.id, i+1, "stdout")
			cmd.Stderr = LogToStdErr(task.id, i+1, "stderr")
		}
		cmd.Dir = r.cfg.Dir
		r.syslog.Write([]byte("Created new task: " + cmd.String() + "\n"))
		slog.Debug("run", "id", task.id, "cmd", cmd.String())

		err = cmd.Run()

		// Check for true errors, else process exit code
		if err == nil {
			break
		}
		if !errors.As(err, &ExEr) {
			slog.Error("unexpected error from cmd.Run", "err", err)
			return
		}
		exitCode = cmd.ProcessState.ExitCode()
		if exitCode == r.cfg.sentinel_code || r.cfg.AbortOnError {
			slog.Debug("job threw sentinel error", "id", task.id, "code", exitCode)
			r.ExitCode = exitCode
			r.cancel()
			break
		}
	}
	if err != nil {
		r.errored.Add(1)

	} else {
		r.completed.Add(1)
	}
}

func (r *Runner) Run(s string) {
	as_split, err := shlex.Split(s)
	if err != nil {
		slog.Error("could not parse as command", "cmd", s)
		return
	}
	next_id := r.count.Add(1)
	task := QueuedAction{
		id:   int(next_id),
		args: as_split,
	}

	r.Add()

	go r.manageTask(&task)
}

// Passthrough to cancel the runner's context
func (r *Runner) Cancel() {
	slog.Debug("cancelling")
	r.cancel()
}

// Wraps add methods for the waitgroup, semaphore, and counter
func (r *Runner) Add() {
	if r.sem == nil {
		// Bypass semaphore use
		r.wg.Add(1)
		return
	}
	select {
	case <-r.ctx.Done():
		return
	case r.sem <- token{}:
		r.wg.Add(1)
		return
	}
}

// Wraps done methods for the waitgroup, semaphore, and counter
func (r *Runner) Done() {
	if r.sem != nil {
		<-r.sem
	}
	r.wg.Done()
}

// Blocks until the waitgroup is finished
func (r *Runner) Wait() int {
	slog.Debug("waiting")
	r.wg.Wait()
	slog.Debug("waitgroup empty")
	r.cancel()
	return r.ExitCode
}

func (r *Runner) PrintBar() {
	total := r.count.Load()
	if total < 1 {
		return
	}
	completed := r.completed.Load()
	errored := r.errored.Load()
	remain := total - completed - errored
	// queued := int64(len(r.Intake))

	slog.Debug("logging", "completed", completed, "errored", errored, "total", total)
	frac_done := (completed * STATUS_COL_W) / total
	frac_remain := (remain * STATUS_COL_W) / total
	frac_errored := (errored * STATUS_COL_W) / total
	// frac_queued := (queued * STATUS_COL_W) / total

	// TODO: Ensure all nonzero numbers have at least one symbol printed

	var remainColor string
	select {
	case <-r.ctx.Done():
		// Any remaining items are cancelled
		remainColor = "\033[0m"
	default:
		remainColor = "\033[34m"
	}

	// TODO: Display errored jobs. Maybe all in order instead of by status?
	line1 := fmt.Sprintf("\033[32m%s", strings.Repeat("=", int(frac_done)))
	line1 += fmt.Sprintf("\033[31m%s", strings.Repeat("|", int(frac_errored)))
	line1 += fmt.Sprintf("%s%s\033[0m", remainColor, strings.Repeat("-", int(frac_remain)))
	// line1 += fmt.Sprintf("\033[0m%s", strings.Repeat(".", int(frac_queued)))

	line1 += fmt.Sprintf("Running: %d Errored: %d Completed: %d", remain, errored, completed)

	// Print overwriting last 2 lines
	// fmt.Println("\033[2A\033[2K\033[G" + line1[:STATUS_COL_W] + "\033[0m")
	fmt.Println(line1)
	// fmt.Println("\033[2K\033[G" + line2)
}
