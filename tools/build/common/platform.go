package common

import (
	"os"
	"os/exec"
)

// Which resolves a command in PATH, returning "" if it is not found.
func Which(cmd string) string {
	p, err := exec.LookPath(cmd)
	if err != nil {
		return ""
	}
	return p
}

// Exists reports whether a path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
