package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBin holds copies of the test binary named rclone and editor. When run under
// those names, TestMain makes the binary act as a fake rclone or editor, so the
// tests work the same on Linux and Windows without shell scripts.
var fakeBin string

func TestMain(m *testing.M) {
	switch strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") {
	case "rclone":
		os.Exit(fakeRclone(os.Args[1:]))
	case "editor":
		os.Exit(fakeEditor(os.Args[1:]))
	}

	dir, err := os.MkdirTemp("", "zsync-test-bin")
	if err != nil {
		panic(err)
	}
	fakeBin = dir
	for _, name := range []string{"rclone", "editor"} {
		if err := copyExecutable(filepath.Join(dir, exeName(name))); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeRclone logs its arguments, one call per line, to $ZSYNC_TEST_LOG.
// lsjson fails for paths containing "missing" and sync fails for paths containing "fail".
func fakeRclone(args []string) int {
	if log, err := os.OpenFile(os.Getenv("ZSYNC_TEST_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(log, strings.Join(args, " "))
		log.Close()
	}
	switch args[0] {
	case "lsjson":
		path := args[2]
		switch {
		case strings.Contains(path, "missing"):
			return 3
		case strings.Contains(path, "file"):
			fmt.Println(`{"Path":"f","IsDir":false}`)
		case strings.HasSuffix(path, "root:"):
		default:
			fmt.Println("{\n  \"Path\": \"d\",\n  \"IsDir\": true\n}")
		}
	case "sync":
		if strings.Contains(args[1]+args[2], "fail") {
			return 1
		}
	}
	return 0
}

// fakeEditor does what $ZSYNC_TEST_EDITOR says with the file given as its last argument.
func fakeEditor(args []string) int {
	path := args[len(args)-1]
	switch os.Getenv("ZSYNC_TEST_EDITOR") {
	case "write-pair": // also requires the extra argument from EDITOR="editor --wait"
		if args[0] != "--wait" {
			return 9
		}
		os.WriteFile(path, []byte("[[pairs]]\nremote = \"gdrive:/x\"\nlocal = \"/x\"\n"), 0o644)
	case "require-file":
		if !isFile(path) {
			return 9
		}
	case "invalid":
		os.WriteFile(path, []byte("not toml [\n"), 0o644)
	case "exit3":
		return 3
	}
	return 0
}

func copyExecutable(dest string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o755)
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// setHome points the user's home directory at a new temporary directory and returns it.
func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // used by os.UserHomeDir on Windows
	return home
}

// setupEnv gives the test its own home directory and puts the fake rclone first on the PATH.
// It returns the path of the log file with the rclone calls.
func setupEnv(t *testing.T) string {
	t.Helper()
	setHome(t)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	log := filepath.Join(t.TempDir(), "rclone.log")
	t.Setenv("ZSYNC_TEST_LOG", log)
	return log
}

// rcloneCalls returns the logged rclone calls, skipping the `rclone version` checks.
func rcloneCalls(t *testing.T, log string) []string {
	t.Helper()
	content, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line = strings.TrimSpace(line); line != "version" {
			calls = append(calls, line)
		}
	}
	return calls
}

// syncCalls returns the logged `rclone sync` calls, in order.
func syncCalls(t *testing.T, log string) []string {
	t.Helper()
	var calls []string
	for _, call := range rcloneCalls(t, log) {
		if strings.HasPrefix(call, "sync ") {
			calls = append(calls, call)
		}
	}
	return calls
}

// useEditor makes the fake editor the configured editor, with the given behavior.
func useEditor(t *testing.T, behavior, args string) string {
	t.Helper()
	editor := filepath.Join(fakeBin, exeName("editor"))
	t.Setenv("ZSYNC_TEST_EDITOR", behavior)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", strings.TrimSpace(editor+" "+args))
	return editor
}

// setInput replaces the prompt input for the duration of the test.
func setInput(t *testing.T, input string) {
	t.Helper()
	old := stdin
	stdin = bufio.NewReader(strings.NewReader(input))
	t.Cleanup(func() { stdin = old })
}

// capture runs fn and returns what it wrote to stdout and stderr.
func capture(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outC, errC := readAll(outR), readAll(errR)
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	err = fn()
	outW.Close()
	errW.Close()
	return <-outC, <-errC, err
}

func readAll(r io.Reader) <-chan string {
	c := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		c <- string(b)
	}()
	return c
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func saveConfig(t *testing.T, pairs ...Pair) {
	t.Helper()
	if err := (&Config{Pairs: pairs}).save(); err != nil {
		t.Fatal(err)
	}
}

// mustLoad loads the configuration with the pairs' Source cleared, for easy comparison.
func mustLoad(t *testing.T) *Config {
	t.Helper()
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	for i := range config.Pairs {
		config.Pairs[i].Source = ""
	}
	return config
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
