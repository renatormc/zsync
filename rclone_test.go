package main

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnsureRcloneInstalled(t *testing.T) {
	setupEnv(t)
	if err := ensureRcloneInstalled(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", t.TempDir())
	err := ensureRcloneInstalled()
	if err == nil {
		t.Fatal("expected an error without rclone on the PATH")
	}
	assertContains(t, err.Error(), "rclone not found")
}

func TestRemoteDirExists(t *testing.T) {
	setupEnv(t)
	tests := map[string]bool{
		"gdrive:/dir":     true,
		"gdrive:root:":    true, // empty output: the root of a remote
		"gdrive:/file":    false,
		"gdrive:/missing": false,
	}
	for remote, want := range tests {
		got, err := remoteDirExists(remote)
		if err != nil {
			t.Fatalf("%s: %v", remote, err)
		}
		if got != want {
			t.Errorf("%s: got %v, want %v", remote, got, want)
		}
	}
}

func TestRcloneSyncArgs(t *testing.T) {
	log := setupEnv(t)
	if err := rcloneSync("/src", "gdrive:/dst", "", false, false); err != nil {
		t.Fatal(err)
	}
	if err := rcloneSync("/src", "gdrive:/dst", "/src/.zsyncignore", true, true); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"sync /src gdrive:/dst",
		"sync /src gdrive:/dst --exclude-from /src/.zsyncignore --progress --dry-run",
	}
	if got := rcloneCalls(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRcloneSyncFailure(t *testing.T) {
	setupEnv(t)
	err := rcloneSync("/src", "gdrive:/fail", "", false, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	assertContains(t, err.Error(), "rclone sync /src -> gdrive:/fail failed")
}

// TestRealRclone syncs between two local folders with the real rclone, if installed.
func TestRealRclone(t *testing.T) {
	if _, err := exec.LookPath("rclone"); err != nil {
		t.Skip("rclone not installed")
	}
	setHome(t)
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src, "keep.txt"), "keep")
	writeFile(t, filepath.Join(src, "skip.log"), "skip")
	writeFile(t, filepath.Join(src, ignoreFile), "*.log\n")
	writeFile(t, filepath.Join(dst, "stale.txt"), "deleted by sync")
	remote := ":local:" + dst

	ok, err := remoteDirExists(remote)
	if err != nil || !ok {
		t.Fatalf("remoteDirExists(%s) = %v, %v", remote, ok, err)
	}
	if ok, _ := remoteDirExists(remote + "/missing"); ok {
		t.Error("missing remote folder reported as existing")
	}

	saveConfig(t, Pair{Remote: remote, Local: src})
	if _, _, err := capture(t, func() error { return cmdSync(up, false, false) }); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"keep.txt": true, ignoreFile: true, "skip.log": false, "stale.txt": false} {
		if got := isFile(filepath.Join(dst, name)); got != want {
			t.Errorf("%s exists = %v, want %v", name, got, want)
		}
	}
}
