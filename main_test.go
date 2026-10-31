package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home := setHome(t)
	tests := map[string]string{
		"~":         home,
		"~/":        home,
		"~/docs":    filepath.Join(home, "docs"),
		"~other/x":  "~other/x",
		"/abs/path": "/abs/path",
		"rel/path":  "rel/path",
		"a/~/b":     "a/~/b",
	}
	if runtime.GOOS == "windows" {
		tests[`~\docs`] = filepath.Join(home, "docs")
	}
	for input, want := range tests {
		if got := expandTilde(input); got != want {
			t.Errorf("expandTilde(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalize(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	os.Mkdir(target, 0o755)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported")
	}
	t.Chdir(dir)

	for _, input := range []string{"target", "./target/", "link", link, "target/../link"} {
		got, err := canonicalize(input)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if got != target {
			t.Errorf("canonicalize(%q) = %q, want %q", input, got, target)
		}
	}
}

func TestPrompt(t *testing.T) {
	setInput(t, "  first  \nsecond\nlast without newline")
	for _, want := range []string{"first", "second", "last without newline"} {
		var got string
		_, _, err := capture(t, func() (err error) {
			got, err = prompt("> ")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if _, _, err := capture(t, func() error { _, err := prompt("> "); return err }); err == nil || err.Error() != "input closed" {
		t.Errorf("expected input closed error, got %v", err)
	}
}

func TestCmdAdd(t *testing.T) {
	log := setupEnv(t)
	local := t.TempDir()
	want, _ := filepath.EvalSymlinks(local)
	setInput(t, "no-colon\ngdrive:/missing\ngdrive:/docs\n/does/not/exist\n"+local+"/\n")

	out, errOut, err := capture(t, cmdAdd)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, errOut, "Invalid format")
	assertContains(t, errOut, "Remote folder not found: gdrive:/missing")
	assertContains(t, errOut, "Local folder not found: /does/not/exist")
	assertContains(t, out, "Added: "+want+" <-> gdrive:/docs")

	if got := mustLoad(t).Pairs; !reflect.DeepEqual(got, []Pair{{Remote: "gdrive:/docs", Local: want}}) {
		t.Errorf("unexpected pairs: %v", got)
	}
	if got := rcloneCalls(t, log); !reflect.DeepEqual(got, []string{"lsjson --stat gdrive:/missing", "lsjson --stat gdrive:/docs"}) {
		t.Errorf("unexpected rclone calls: %q", got)
	}
}

func TestCmdAddTilde(t *testing.T) {
	setupEnv(t)
	home, _ := os.UserHomeDir()
	os.Mkdir(filepath.Join(home, "docs"), 0o755)
	want, _ := filepath.EvalSymlinks(filepath.Join(home, "docs"))
	setInput(t, "gdrive:/docs\n~/docs\n")

	if _, _, err := capture(t, cmdAdd); err != nil {
		t.Fatal(err)
	}
	if got := mustLoad(t).Pairs[0].Local; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCmdAddDuplicate(t *testing.T) {
	setupEnv(t)
	local, _ := filepath.EvalSymlinks(t.TempDir())
	saveConfig(t, Pair{Remote: "gdrive:/docs", Local: local})
	setInput(t, "gdrive:/docs\n"+local+"\n")

	_, _, err := capture(t, cmdAdd)
	if err == nil || err.Error() != "this folder pair is already registered" {
		t.Errorf("expected duplicate error, got %v", err)
	}
	if n := len(mustLoad(t).Pairs); n != 1 {
		t.Errorf("expected 1 pair, got %d", n)
	}
}

func TestCmdAddInputClosed(t *testing.T) {
	setupEnv(t)
	setInput(t, "gdrive:/docs\n")
	if _, _, err := capture(t, cmdAdd); err == nil || err.Error() != "input closed" {
		t.Errorf("expected input closed error, got %v", err)
	}
}

func TestCmdRemove(t *testing.T) {
	setupEnv(t)
	a := Pair{Remote: "gdrive:/a", Local: "/a"}
	b := Pair{Remote: "gdrive:/b", Local: "/b"}
	c := Pair{Remote: "gdrive:/c", Local: "/c"}
	saveConfig(t, a, b, c)
	setInput(t, "0\n4\nx\n2\n")

	out, errOut, err := capture(t, cmdRemove)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(errOut, "Enter a number between 1 and 3."); n != 3 {
		t.Errorf("expected 3 invalid-input messages, got %d:\n%s", n, errOut)
	}
	assertContains(t, out, "  2. /b <-> gdrive:/b")
	assertContains(t, out, "Removed: /b <-> gdrive:/b")
	if got := mustLoad(t).Pairs; !reflect.DeepEqual(got, []Pair{a, c}) {
		t.Errorf("unexpected pairs: %v", got)
	}
}

func TestCmdRemoveCancel(t *testing.T) {
	setupEnv(t)
	saveConfig(t, Pair{Remote: "gdrive:/a", Local: "/a"})
	setInput(t, "\n")

	out, _, err := capture(t, cmdRemove)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Cancelled.")
	if n := len(mustLoad(t).Pairs); n != 1 {
		t.Errorf("expected 1 pair, got %d", n)
	}
}

func TestCmdRemoveEmpty(t *testing.T) {
	setupEnv(t)
	out, _, err := capture(t, cmdRemove)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "No folders registered.")
}

func TestCmdList(t *testing.T) {
	setupEnv(t)
	out, _, err := capture(t, cmdList)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "No folders registered. Use `zsync add` to add one.")

	saveConfig(t, Pair{Remote: "gdrive:/a", Local: "/a"}, Pair{Remote: "gdrive:/b", Local: "/b"})
	out, _, err = capture(t, cmdList)
	if err != nil {
		t.Fatal(err)
	}
	if want := "  1. /a <-> gdrive:/a\n  2. /b <-> gdrive:/b\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestCmdSync(t *testing.T) {
	log := setupEnv(t)
	withIgnore, plain := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(withIgnore, ignoreFile), "*.log\n")
	os.Mkdir(filepath.Join(plain, ignoreFile), 0o755) // a directory, not an ignore file
	saveConfig(t, Pair{Remote: "gdrive:/a", Local: withIgnore}, Pair{Remote: "gdrive:/b", Local: plain})
	ignorePath := filepath.Join(withIgnore, ignoreFile)

	out, _, err := capture(t, func() error { return cmdSync(up, true, false) })
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "==> "+withIgnore+" -> gdrive:/a")
	assertContains(t, out, "    using excludes from "+ignorePath)
	assertContains(t, out, "Sync complete.")

	_, _, err = capture(t, func() error { return cmdSync(down, false, true) })
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"sync " + withIgnore + " gdrive:/a --exclude-from " + ignorePath + " --progress",
		"sync " + plain + " gdrive:/b --progress",
		"sync gdrive:/a " + withIgnore + " --exclude-from " + ignorePath + " --dry-run",
		"sync gdrive:/b " + plain + " --dry-run",
	}
	if got := syncCalls(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestCmdSyncContinuesAfterFailure(t *testing.T) {
	log := setupEnv(t)
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	saveConfig(t,
		Pair{Remote: "gdrive:/fail1", Local: a},
		Pair{Remote: "gdrive:/ok", Local: b},
		Pair{Remote: "gdrive:/fail2", Local: c},
	)

	out, errOut, err := capture(t, func() error { return cmdSync(up, false, false) })
	if err == nil || err.Error() != "2 of 3 syncs failed" {
		t.Errorf("expected 2 of 3 failures, got %v", err)
	}
	if strings.Contains(out, "Sync complete.") {
		t.Error("printed success despite failures")
	}
	assertContains(t, errOut, "Error: rclone sync "+a+" -> gdrive:/fail1 failed")
	if n := len(syncCalls(t, log)); n != 3 {
		t.Errorf("expected 3 syncs, got %d", n)
	}
}

func TestCmdSyncEmpty(t *testing.T) {
	log := setupEnv(t)
	out, _, err := capture(t, func() error { return cmdSync(up, false, false) })
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "No folders registered.")
	if calls := rcloneCalls(t, log); len(calls) != 0 {
		t.Errorf("unexpected rclone calls: %q", calls)
	}
}

func TestCmdSyncWithoutRclone(t *testing.T) {
	setupEnv(t)
	t.Setenv("PATH", t.TempDir())
	if err := cmdSync(up, false, false); err == nil {
		t.Fatal("expected an error without rclone")
	}
}

func TestCmdCheck(t *testing.T) {
	setupEnv(t)
	local := t.TempDir()
	saveConfig(t, Pair{Remote: "gdrive:/ok", Local: local})

	out, _, err := capture(t, cmdCheck)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "     local:  ok\n     remote: ok\n")
	assertContains(t, out, "All folders exist.")

	saveConfig(t,
		Pair{Remote: "gdrive:/ok", Local: local},
		Pair{Remote: "gdrive:/missing", Local: "/does/not/exist"},
		Pair{Remote: "gdrive:/missing2", Local: local},
	)
	out, _, err = capture(t, cmdCheck)
	if err == nil || err.Error() != "3 folder(s) not found" {
		t.Errorf("expected 3 missing folders, got %v", err)
	}
	assertContains(t, out, "  2. /does/not/exist <-> gdrive:/missing\n     local:  NOT FOUND\n     remote: NOT FOUND\n")
}

func TestCmdConfig(t *testing.T) {
	setupEnv(t)
	useEditor(t, "write-pair", "--wait")

	if _, _, err := capture(t, cmdConfig); err != nil {
		t.Fatal(err)
	}
	if got := mustLoad(t).Pairs; !reflect.DeepEqual(got, []Pair{{Remote: "gdrive:/x", Local: "/x"}}) {
		t.Errorf("unexpected pairs: %v", got)
	}
}

func TestCmdConfigCreatesFileAndPrefersVisual(t *testing.T) {
	setupEnv(t)
	editor := useEditor(t, "require-file", "")
	t.Setenv("VISUAL", editor)
	t.Setenv("EDITOR", "/does/not/exist")

	if _, _, err := capture(t, cmdConfig); err != nil {
		t.Fatal(err)
	}
}

func TestCmdConfigErrors(t *testing.T) {
	setupEnv(t)

	useEditor(t, "invalid", "")
	_, _, err := capture(t, cmdConfig)
	if err == nil {
		t.Fatal("expected an invalid configuration error")
	}
	assertContains(t, err.Error(), "the configuration has errors")

	useEditor(t, "exit3", "")
	_, _, err = capture(t, cmdConfig)
	if err == nil {
		t.Fatal("expected an editor exit error")
	}
	assertContains(t, err.Error(), "exited with exit status 3")

	t.Setenv("EDITOR", "/does/not/exist")
	_, _, err = capture(t, cmdConfig)
	if err == nil {
		t.Fatal("expected an editor start error")
	}
	assertContains(t, err.Error(), "failed to start editor")
}

func TestEditorCommand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Program Files")
	os.Mkdir(dir, 0o755)
	withSpaces := filepath.Join(dir, "my editor.exe")
	writeFile(t, withSpaces, "")

	tests := map[string][]string{
		"vi":                          {"vi"},
		"code --wait":                 {"code", "--wait"},
		"  nano  -w ":                 {"nano", "-w"},
		withSpaces:                    {withSpaces},
		`"` + withSpaces + `"`:        {withSpaces},
		`"` + withSpaces + `" --wait`: {withSpaces, "--wait"},
	}
	for input, want := range tests {
		if got := editorCommand(input); !reflect.DeepEqual(got, want) {
			t.Errorf("editorCommand(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRootCmd(t *testing.T) {
	setupEnv(t)
	tests := []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"list"}, false},
		{[]string{"up", "-n", "-p"}, false},
		{[]string{"down", "--dry-run", "--progress"}, false},
		{[]string{"list", "extra"}, true},
		{[]string{"up", "--bogus"}, true},
		{[]string{"nope"}, true},
	}
	for _, tt := range tests {
		cmd := newRootCmd()
		cmd.SetArgs(tt.args)
		_, _, err := capture(t, cmd.Execute)
		if (err != nil) != tt.wantErr {
			t.Errorf("%v: err = %v, wantErr %v", tt.args, err, tt.wantErr)
		}
	}
}

func TestCmdSyncAbortsWhenFoldersAreMissing(t *testing.T) {
	for _, dir := range []direction{up, down} {
		log := setupEnv(t)
		ok := t.TempDir()
		missingLocal := filepath.Join(t.TempDir(), "missing-local")
		saveConfig(t,
			Pair{Remote: "gdrive:/ok", Local: ok},
			Pair{Remote: "gdrive:/missing", Local: ok},
			Pair{Remote: "gdrive:/also-ok", Local: missingLocal},
		)

		out, errOut, err := capture(t, func() error { return cmdSync(dir, false, false) })
		if err == nil || err.Error() != "2 folder(s) not found, nothing was synced" {
			t.Errorf("expected an abort, got %v", err)
		}
		if calls := syncCalls(t, log); len(calls) != 0 {
			t.Errorf("synced despite missing folders: %q", calls)
		}
		if out != "" {
			t.Errorf("unexpected output: %q", out)
		}
		want := "Some registered folders do not exist:\n" +
			"  2. " + ok + " <-> gdrive:/missing\n" +
			"     remote folder not found: gdrive:/missing\n" +
			"  3. " + missingLocal + " <-> gdrive:/also-ok\n" +
			"     local folder not found:  " + missingLocal + "\n"
		if errOut != want {
			t.Errorf("got:\n%s\nwant:\n%s", errOut, want)
		}
	}
}

func TestCmdSyncCheckIsSilent(t *testing.T) {
	log := setupEnv(t)
	local := t.TempDir()
	saveConfig(t, Pair{Remote: "gdrive:/ok", Local: local})

	out, errOut, err := capture(t, func() error { return cmdSync(up, false, false) })
	if err != nil {
		t.Fatal(err)
	}
	if want := "==> " + local + " -> gdrive:/ok\nSync complete.\n"; out != want || errOut != "" {
		t.Errorf("got stdout %q, stderr %q; want stdout %q", out, errOut, want)
	}
	if got := rcloneCalls(t, log); !reflect.DeepEqual(got, []string{"lsjson --stat gdrive:/ok", "sync " + local + " gdrive:/ok"}) {
		t.Errorf("unexpected rclone calls: %q", got)
	}
}

func TestCheckFoldersInParallelKeepsOrder(t *testing.T) {
	setupEnv(t)
	local := t.TempDir()
	var pairs []Pair
	for i := range 20 {
		remote := fmt.Sprintf("gdrive:/dir%d", i)
		if i%3 == 0 {
			remote = fmt.Sprintf("gdrive:/missing%d", i)
		}
		pairs = append(pairs, Pair{Remote: remote, Local: local})
	}
	statuses, err := checkFolders(pairs)
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range statuses {
		if !st.localOK || st.remoteOK != (i%3 != 0) {
			t.Errorf("pair %d: got %+v", i, st)
		}
	}
}
