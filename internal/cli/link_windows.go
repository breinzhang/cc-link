//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func createLink(it LinkItem, opts Options) (string, error) {
	st, err := os.Stat(it.Source)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		if err := createJunction(it.Source, it.Target); err == nil {
			return "junction", nil
		}
		return createSymlinkFallback(it)
	}
	if err := os.Link(it.Source, it.Target); err == nil {
		return "hardlink", nil
	}
	return createSymlinkFallback(it)
}

func createJunction(source, target string) error {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", target, absSource)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create junction failed: %s -> %s: %w: %s", target, absSource, err, string(out))
	}
	return nil
}

func createSymlinkFallback(it LinkItem) (string, error) {
	if err := os.Symlink(it.Source, it.Target); err != nil {
		return "", fmt.Errorf("create link failed: %s -> %s: %w\nTried junction/hardlink first, then symlink. On Windows symlink fallback requires Developer Mode or Administrator.", it.Target, it.Source, err)
	}
	return "symlink", nil
}
