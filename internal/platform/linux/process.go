package linux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// ExecRunner runs commands through os/exec without a shell.
type ExecRunner struct{}

// Run executes path with args and returns combined command output.
func (ExecRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, path, args...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w", path, err)
	}
	return output.Bytes(), nil
}
