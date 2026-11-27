//go:build !windows

package engine

import (
	"context"
	"os/exec"
)

func newCoreCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, path, args...)
}
