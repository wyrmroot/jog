package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wyrmroot/jog/runner"
)

/*
Generates a function with a interface suitable for passing into bufio.Scanner.Split()
*/
func makeSplitter(delim string) func([]byte, bool) (int, []byte, error) {
	delim_b := []byte(delim)
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// Return nothing if at end of file and no data passed
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.Index(data, delim_b); i >= 0 {
			return i + 1, data[0:i], nil
		}
		// If at end of file with data return the data
		if atEOF {
			return len(data), data, nil
		}
		return
	}
}

// Reads a file-like object, splitting on delim, into an unbuffered channel for consumption.
func streamFrom(filelike *os.File, delim string) chan string {
	out := make(chan string)
	scanner := bufio.NewScanner(filelike)
	scanner.Split(makeSplitter(delim))
	go func() {
		for scanner.Scan() {
			// slog.Debug("scanner reading text")
			t := scanner.Text()
			out <- t
		}
		close(out)
		slog.Debug("input scanner done")
	}()
	return out
}

/*
Takes a template: 'echo value:{}'
replacement string: {}
and channel containing individual values

Returns a channel containing finished commands
*/
func makeReplacer(template, pat string, in chan string) chan string {
	var mergeFunc func(string) string
	switch {
	case template == "":
		// With no command template given, execute each s as its own cmd
		slog.Debug("no command template in use")
		mergeFunc = func(s string) string {
			return s
		}
	case pat != "" && strings.Contains(template, pat):
		slog.Debug("using ReplaceAll to replace pattern " + pat)
		mergeFunc = func(s string) string {
			return strings.ReplaceAll(template, pat, s)
		}
	default:
		// If pat isn't in template, we will append instead of substitute
		slog.Debug("pattern not in template, appending args at the end")
		mergeFunc = func(s string) string {
			return template + " " + s
		}
	}
	out := make(chan string)
	go func() {
		for s := range in {
			out <- mergeFunc(s)
		}
		close(out)
	}()
	return out
}

func main() {
	// UX
	f_progress := flag.Bool("progress", false, "display progress bar to stdout")
	f_debug := flag.Bool("debug", false, "include debug output")

	// Run params
	f_jobs := flag.Uint("j", 0, "maximum number of concurrent jobs to run (default 0 = unlimited)")
	f_dir := flag.String("dir", "", "execute commands from the given directory (default .)")
	f_argfile := flag.String("f", "", "read command file for items instead of stdin")

	// Arguments, delimiters and substitutions
	f_delim := flag.String("d", "\n", "command delimiter (default newline)")
	f_null := flag.Bool("null", false, "use null byte as the delimiter (overwrites -d)")
	f_replace := flag.String("replace", "{}", "replace instances of str with args read from stdin. Default {}, disable by setting to an empty string")

	// Error handling
	f_exit := flag.Bool("exit", false, "cancel all jobs upon any job reaching an error (overwrites -k)")
	f_attempts := flag.Uint("attempts", 1, "maximum number of attempts per task (default 1)")
	f_mindelay := flag.Uint("min-delay", 0, "minimum wait time (ms) between attempts")
	f_maxdelay := flag.Uint("max-delay", 1, "maximum wait time (ms) between attempts")
	// f_killcode := flag.Int("k", 255, "if a process exits with this code, cancels all others and exits")

	f_log := flag.Bool("log", false, "print structured logs for all processes to stderr")

	// Parse all arguments
	flag.Parse()
	if *f_debug {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)
	}
	// Null byte separator
	if *f_null {
		*f_delim = "\000"
	}

	// Create a stream from either stdin or a file
	var arg_source *os.File
	if *f_argfile != "" {
		slog.Debug("opening arg file")
		var err error
		arg_source, err = os.Open(*f_argfile)
		if err != nil {
			slog.Error("unable to open arg file " + *f_argfile)
			os.Exit(126)
		}
		defer arg_source.Close()
	} else {
		slog.Debug("getting cmds from stdin")
		arg_source = os.Stdin
	}
	argCh := streamFrom(arg_source, *f_delim)

	var template string
	if len(flag.Args()) > 0 {
		template = strings.Join(flag.Args(), " ")
		slog.Debug("have command template: " + template)
	}
	readCh := makeReplacer(template, *f_replace, argCh)

	// Create the runner
	cfg := runner.RunnerConfig{
		Attempts:     max(1, *f_attempts),
		AbortOnError: *f_exit,
		Concurrency:  *f_jobs,
		PrintStatus:  *f_progress,
		Dir:          *f_dir,
		Log:          *f_log,
		MinDelay:     *f_mindelay,
		MaxDelay:     *f_maxdelay,
	}

	R := runner.NewRunner(&cfg)

	// Listen for interrupts
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	go func() {
		<-sigs
		slog.Warn("manually canceled")
		R.Cancel()
	}()

	// Execute all commands
	for c := range readCh {
		R.Run(c)
	}

	R.Wait()
	if *f_progress {
		// Final status bar
		R.PrintBar()
		fmt.Println()
	}
	slog.Debug("end")

}
