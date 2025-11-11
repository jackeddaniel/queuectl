package worker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecutionResult holds the result of command execution
type ExecutionResult struct {
	ExitCode int
	Output   string
	StdErr   string
	Error    error
}

// ExecuteCommand executes a shell command with timeout
func ExecuteCommand(ctx context.Context, command string, timeout time.Duration) ExecutionResult {
	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse command
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ExecutionResult{
			ExitCode: 127,
			Error:    fmt.Errorf("empty command"),
		}
	}

	// Create command
	cmd := exec.CommandContext(cmdCtx, parts[0], parts[1:]...)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set environment
	cmd.Env = os.Environ()

	// Execute command
	err := cmd.Run()

	// Extract exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return ExecutionResult{
		ExitCode: exitCode,
		Output:   stdout.String(),
		StdErr:   stderr.String(),
		Error:    err,
	}
}

// IsSuccess returns true if the command succeeded (exit code 0)
func (r ExecutionResult) IsSuccess() bool {
	return r.ExitCode == 0 && r.Error == nil
}

// GetFullOutput returns combined stdout and stderr
func (r ExecutionResult) GetFullOutput() string {
	combined := r.Output
	if r.StdErr != "" {
		combined += "\nSTDERR:\n" + r.StdErr
	}
	return combined
}
