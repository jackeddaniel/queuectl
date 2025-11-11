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

	// Parse command - use shell to handle complex commands
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)

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
		} else if cmdCtx.Err() == context.DeadlineExceeded {
			exitCode = 124 // Timeout exit code
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
		if combined != "" {
			combined += "\n"
		}
		combined += "STDERR:\n" + r.StdErr
	}
	return combined
}

// GetErrorMessage returns a descriptive error message
func (r ExecutionResult) GetErrorMessage() string {
	if r.Error != nil {
		if strings.Contains(r.Error.Error(), "deadline exceeded") {
			return fmt.Sprintf("command timed out (exit %d)", r.ExitCode)
		}
		return fmt.Sprintf("command failed: %v (exit %d)", r.Error, r.ExitCode)
	}
	if r.ExitCode != 0 {
		return fmt.Sprintf("command exited with code %d", r.ExitCode)
	}
	return ""
}