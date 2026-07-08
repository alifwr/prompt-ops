package shell

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type CommandResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// RunCommand executes a command on the host with a timeout duration
func RunCommand(timeout time.Duration, name string, args ...string) *CommandResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	// Capturing outputs using standard output buffers
	var stdoutBuf, stderrBuf stringsBuilder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return &CommandResult{ExitCode: -1, Error: err.Error()}
	}

	err := cmd.Wait()

	exitCode := 0
	errMsg := ""
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			errMsg = err.Error()
		}
	}

	return &CommandResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Error:    errMsg,
	}
}

// stringsBuilder helper since strings.Builder is not thread-safe but safe for single-threaded command outputs
type stringsBuilder struct {
	builder strings.Builder
}

func (sb *stringsBuilder) Write(p []byte) (n int, err error) {
	return sb.builder.Write(p)
}

func (sb *stringsBuilder) String() string {
	return sb.builder.String()
}
