package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ensureRcloneInstalled ensures the rclone executable is available on the PATH.
func ensureRcloneInstalled() error {
	if err := exec.Command("rclone", "version").Run(); err != nil {
		return fmt.Errorf("rclone not found. Install it from https://rclone.org/install/ and set up your remotes with `rclone config`")
	}
	return nil
}

// remoteDirExists checks whether the remote path exists and is a directory.
func remoteDirExists(remote string) (bool, error) {
	out, err := exec.Command("rclone", "lsjson", "--stat", remote).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to run rclone: %w", err)
	}
	// The root of a remote (e.g. "gdrive:") may return nothing; success is enough in that case.
	compact := strings.Join(strings.Fields(string(out)), "")
	return compact == "" || strings.Contains(compact, `"IsDir":true`), nil
}

// rcloneSync runs `rclone sync <source> <dest>`.
func rcloneSync(source, dest, excludeFrom string, progress, dryRun bool) error {
	args := []string{"sync", source, dest}
	if excludeFrom != "" {
		args = append(args, "--exclude-from", excludeFrom)
	}
	if progress {
		args = append(args, "--progress")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	cmd := exec.Command("rclone", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("rclone sync %s -> %s failed (%w)", source, dest, err)
		}
		return fmt.Errorf("failed to run rclone: %w", err)
	}
	return nil
}
