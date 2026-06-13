//go:build !windows

package cli

import (
	"fmt"
	"os"
)

func createLink(it LinkItem, opts Options) (string, error) {
	if err := os.Symlink(it.Source, it.Target); err != nil {
		return "", fmt.Errorf("create symlink failed: %s -> %s: %w", it.Target, it.Source, err)
	}
	return "symlink", nil
}
