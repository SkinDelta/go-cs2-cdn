package cmdrunner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// CommandResult holds the results of an executed command.
type CommandResult struct {
	Stdout string
	Stderr string
	Error  error
}

// Runner defines an interface for running commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) CommandResult
}

// DefaultRunner is the default implementation of Runner.
type DefaultRunner struct{}

// Run executes the command with the given name and arguments.
// It captures stdout and stderr, respects the provided context for timeouts and cancellations.
func (r *DefaultRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Printf("Executing command: %s %s", name, strings.Join(args, " "))

	err := cmd.Run()

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()

	if stdoutStr != "" {
		log.Printf("Command stdout: %s", stdoutStr)
	}
	if stderrStr != "" {
		log.Printf("Command stderr: %s", stderrStr)
	}

	if err != nil {
		// Check if the error is due to context timeout
		if ctx.Err() == context.DeadlineExceeded {
			return CommandResult{
				Stdout: stdoutStr,
				Stderr: stderrStr,
				Error:  fmt.Errorf("command timed out: %w", err),
			}
		}
		return CommandResult{
			Stdout: stdoutStr,
			Stderr: stderrStr,
			Error:  fmt.Errorf("command execution failed: %w", err),
		}
	}

	return CommandResult{
		Stdout: stdoutStr,
		Stderr: stderrStr,
		Error:  nil,
	}
}

// RunCommand is a helper function to execute a command with a default timeout.
func RunCommand(name string, args ...string) CommandResult {
	runner := &DefaultRunner{}
	// Define a timeout duration, e.g., 60 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return runner.Run(ctx, name, args...)
}

// PipeOutput streams the stdout and stderr of a command in real-time.
func PipeOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Printf("[stdout] %s", scanner.Text())
		}
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[stderr] %s", scanner.Text())
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}
