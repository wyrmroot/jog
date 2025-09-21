package runner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

func NewSysLogger() *io.PipeWriter {
	// Create a writer to give to the command and a reader to use here
	pr, pw := io.Pipe()
	// Connect reader to a scanner to break on newlines
	s := bufio.NewScanner(pr)
	// Continuously read the scanner into stderr
	go func() {
		for s.Scan() {
			fmt.Fprintf(os.Stderr, `{"time": "%d", "type": "system", "msg": %s}`+"\n", time.Now().UnixMilli(), strconv.Quote(s.Text()))
			os.Stderr.Sync()
		}
	}()

	return pw
}

type NullLogger struct{}

func (n *NullLogger) Write([]byte) (int, error) {
	return 0, nil
}
func (n *NullLogger) Close() {}

func LogToStdErr(pid, attempt int, logtype string) *io.PipeWriter {
	// Create a writer to give to the command and a reader to use here
	pr, pw := io.Pipe()
	// Connect reader to a scanner to break on newlines
	s := bufio.NewScanner(pr)
	// Continuously read the scanner into stderr
	go func() {
		for s.Scan() {
			fmt.Fprintf(os.Stderr, `{"time": %d, "line": "%d", "attempt": "%d", "type": "%s", "msg": %s}`+"\n", time.Now().UnixMilli(), pid, attempt, logtype, strconv.Quote(s.Text()))
			os.Stderr.Sync()
		}
	}()

	return pw
}
