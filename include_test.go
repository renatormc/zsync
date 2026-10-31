package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// writeConfig writes a configuration file, creating its folder, and returns its path.
func writeConfig(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, content)
	return path
}

func mainConfigPath(t *testing.T) string {
	t.Helper()
	path, err := configFile()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIncludeLoadsPairsFromAllFiles(t *testing.T) {
	home := setHome(t)
	main := mainConfigPath(t)
	shared := filepath.Join(t.TempDir(), "shared.toml")
	work := writeConfig(t, filepath.Join(home, ".zsync", "work.toml"), `include = ["sub/nested.toml"]

[[pairs]]
remote = "gdrive:/work"
local = "/work"
`)
	nested := writeConfig(t, filepath.Join(home, ".zsync", "sub", "nested.toml"), `[[pairs]]
remote = "gdrive:/nested"
local = "/nested"
`)
	writeConfig(t, shared, "[[pairs]]\nremote = \"gdrive:/shared\"\nlocal = \"/shared\"\n")
	writeConfig(t, filepath.Join(home, "home.toml"), "[[pairs]]\nremote = \"gdrive:/home\"\nlocal = \"/home\"\n")
	// Relative to ~/.zsync, absolute, and with ~.
	writeConfig(t, main, `include = ["work.toml", "`+filepath.ToSlash(shared)+`", "~/home.toml"]

[[pairs]]
remote = "gdrive:/main"
local = "/main"
`)

	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := []Pair{
		{Remote: "gdrive:/main", Local: "/main", Source: main},
		{Remote: "gdrive:/work", Local: "/work", Source: work},
		{Remote: "gdrive:/nested", Local: "/nested", Source: nested},
		{Remote: "gdrive:/shared", Local: "/shared", Source: shared},
		{Remote: "gdrive:/home", Local: "/home", Source: filepath.Join(home, "home.toml")},
	}
	if !reflect.DeepEqual(config.Pairs, want) {
		t.Errorf("got  %v\nwant %v", config.Pairs, want)
	}
}

func TestIncludeCycleAndDuplicateLoadedOnce(t *testing.T) {
	home := setHome(t)
	dir := filepath.Join(home, ".zsync")
	writeConfig(t, filepath.Join(dir, "config.toml"), `include = ["a.toml", "b.toml", "./a.toml"]`)
	writeConfig(t, filepath.Join(dir, "a.toml"), "include = [\"config.toml\", \"b.toml\"]\n[[pairs]]\nremote = \"r:/a\"\nlocal = \"/a\"\n")
	writeConfig(t, filepath.Join(dir, "b.toml"), "include = [\"a.toml\"]\n[[pairs]]\nremote = \"r:/b\"\nlocal = \"/b\"\n")

	want := []Pair{{Remote: "r:/a", Local: "/a"}, {Remote: "r:/b", Local: "/b"}}
	if got := mustLoad(t).Pairs; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestIncludeErrors(t *testing.T) {
	tests := map[string]struct {
		included, wantErr string
	}{
		"missing file": {"", "included file not found: "},
		"invalid toml": {"[[pairs]\n", "invalid configuration file: "},
		"empty path":   {`include = [""]`, "`include` has an empty path"},
		"not a list":   {`include = "x.toml"`, "invalid configuration file: "},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			home := setHome(t)
			writeConfig(t, mainConfigPath(t), `include = ["other.toml"]`)
			if tt.included != "" {
				writeConfig(t, filepath.Join(home, ".zsync", "other.toml"), tt.included)
			}
			_, err := loadConfig()
			if err == nil {
				t.Fatal("expected an error")
			}
			assertContains(t, err.Error(), tt.wantErr)
			assertContains(t, err.Error(), "other.toml")
		})
	}
}

func TestIncludeKeptWhenSaving(t *testing.T) {
	setHome(t)
	saveConfig(t, Pair{Remote: "r:/a", Local: "/a"})
	main := mainConfigPath(t)
	if err := updateConfigFile(main, func(c *Config) error {
		c.Include = []string{"other.toml"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(main)
	want := "include = [\"other.toml\"]\n\n[[pairs]]\nremote = \"r:/a\"\nlocal = \"/a\"\n"
	if string(content) != want {
		t.Errorf("got:\n%s\nwant:\n%s", content, want)
	}
}

func TestCmdAddWritesToMainFileOnly(t *testing.T) {
	setupEnv(t)
	home, _ := os.UserHomeDir()
	existing, _ := filepath.EvalSymlinks(t.TempDir())
	other := writeConfig(t, filepath.Join(home, ".zsync", "other.toml"),
		"[[pairs]]\nremote = \"gdrive:/other\"\nlocal = '"+existing+"'\n")
	writeConfig(t, mainConfigPath(t), `include = ["other.toml"]`)
	local, _ := filepath.EvalSymlinks(t.TempDir())

	// A pair already defined in an included file is a duplicate.
	setInput(t, "gdrive:/other\n"+existing+"\n")
	if _, _, err := capture(t, cmdAdd); err == nil || err.Error() != "this folder pair is already registered" {
		t.Errorf("expected a duplicate error for a pair from an included file, got %v", err)
	}

	setInput(t, "gdrive:/new\n"+local+"\n")
	if _, _, err := capture(t, cmdAdd); err != nil {
		t.Fatal(err)
	}
	main, _ := readConfigFile(mainConfigPath(t))
	if !reflect.DeepEqual(main.Include, []string{"other.toml"}) {
		t.Errorf("include changed: %v", main.Include)
	}
	if !reflect.DeepEqual(main.Pairs, []Pair{{Remote: "gdrive:/new", Local: local}}) {
		t.Errorf("unexpected main pairs: %v", main.Pairs)
	}
	if included, _ := readConfigFile(other); len(included.Pairs) != 1 {
		t.Errorf("included file changed: %v", included.Pairs)
	}
}

func TestCmdRemoveFromIncludedFile(t *testing.T) {
	setupEnv(t)
	home, _ := os.UserHomeDir()
	other := writeConfig(t, filepath.Join(home, ".zsync", "other.toml"), `[[pairs]]
remote = "gdrive:/b"
local = "/b"

[[pairs]]
remote = "gdrive:/c"
local = "/c"
`)
	writeConfig(t, mainConfigPath(t), "include = [\"other.toml\"]\n\n[[pairs]]\nremote = \"gdrive:/a\"\nlocal = \"/a\"\n")
	setInput(t, "3\n")

	out, _, err := capture(t, cmdRemove)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "  1. /a <-> gdrive:/a\n")
	assertContains(t, out, "  3. /c <-> gdrive:/c  (from "+other+")\n")
	assertContains(t, out, "Removed: /c <-> gdrive:/c\n    (from "+other+")")

	main, _ := readConfigFile(mainConfigPath(t))
	if !reflect.DeepEqual(main.Include, []string{"other.toml"}) || len(main.Pairs) != 1 {
		t.Errorf("main file changed: %+v", main)
	}
	included, _ := readConfigFile(other)
	if !reflect.DeepEqual(included.Pairs, []Pair{{Remote: "gdrive:/b", Local: "/b"}}) {
		t.Errorf("unexpected included pairs: %v", included.Pairs)
	}
}

func TestCmdSyncAndListUseIncludedPairs(t *testing.T) {
	log := setupEnv(t)
	home, _ := os.UserHomeDir()
	a, b := t.TempDir(), t.TempDir()
	other := writeConfig(t, filepath.Join(home, ".zsync", "other.toml"), "[[pairs]]\nremote = \"gdrive:/b\"\nlocal = '"+b+"'\n")
	writeConfig(t, mainConfigPath(t), "include = [\"other.toml\"]\n\n[[pairs]]\nremote = \"gdrive:/a\"\nlocal = '"+a+"'\n")

	out, _, err := capture(t, cmdList)
	if err != nil {
		t.Fatal(err)
	}
	if want := "  1. " + a + " <-> gdrive:/a\n  2. " + b + " <-> gdrive:/b  (from " + other + ")\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}

	if _, _, err := capture(t, func() error { return cmdSync(up, false, false) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(syncCalls(t, log), "\n"); got != "sync "+a+" gdrive:/a\nsync "+b+" gdrive:/b" {
		t.Errorf("unexpected rclone calls:\n%s", got)
	}
}

func TestCmdAddConfigFile(t *testing.T) {
	home := setHome(t)
	saveConfig(t, Pair{Remote: "gdrive:/a", Local: "/a"})
	other := writeConfig(t, filepath.Join(t.TempDir(), "other.toml"), "[[pairs]]\nremote = \"gdrive:/b\"\nlocal = \"/b\"\n")
	writeConfig(t, filepath.Join(home, "mine.toml"), "")

	// From an argument, relative to the current folder.
	t.Chdir(filepath.Dir(other))
	out, _, err := capture(t, func() error { return cmdAddConfigFile("other.toml") })
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Included: "+other+" (1 pair(s))")

	// From the prompt, with ~.
	setInput(t, "~/mine.toml\n")
	if _, _, err := capture(t, func() error { return cmdAddConfigFile("") }); err != nil {
		t.Fatal(err)
	}

	main, _ := readConfigFile(mainConfigPath(t))
	if want := []string{other, filepath.Join(home, "mine.toml")}; !reflect.DeepEqual(main.Include, want) {
		t.Errorf("got includes %v, want %v", main.Include, want)
	}
	want := []Pair{{Remote: "gdrive:/a", Local: "/a"}, {Remote: "gdrive:/b", Local: "/b"}}
	if got := mustLoad(t).Pairs; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCmdAddConfigFileCreatesMissingFile(t *testing.T) {
	setHome(t)
	path := filepath.Join(t.TempDir(), "new", "shared.toml")

	setInput(t, "n\n")
	out, _, err := capture(t, func() error { return cmdAddConfigFile(path) })
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Cancelled.")
	if isFile(path) {
		t.Error("file created despite answering no")
	}

	setInput(t, "Y\n")
	if _, _, err := capture(t, func() error { return cmdAddConfigFile(path) }); err != nil {
		t.Fatal(err)
	}
	if config, err := readConfigFile(path); err != nil || len(config.Pairs) != 0 {
		t.Errorf("new file not a valid empty configuration: %v, %v", config, err)
	}
	if main, _ := readConfigFile(mainConfigPath(t)); !reflect.DeepEqual(main.Include, []string{path}) {
		t.Errorf("unexpected includes: %v", main.Include)
	}
}

func TestCmdAddConfigFileErrors(t *testing.T) {
	home := setHome(t)
	main := mainConfigPath(t)
	dir := t.TempDir()
	included := writeConfig(t, filepath.Join(dir, "included.toml"), "")
	invalid := writeConfig(t, filepath.Join(dir, "invalid.toml"), "[[pairs]\n")
	brokenInclude := writeConfig(t, filepath.Join(dir, "broken.toml"), `include = ["nope.toml"]`)
	writeConfig(t, main, `include = ["`+filepath.ToSlash(included)+`"]`)
	before, _ := os.ReadFile(main)

	tests := map[string]struct{ input, wantErr string }{
		"main file":        {main, "that is the main configuration file"},
		"already included": {included, "is already included"},
		"folder":           {home, "is a folder"},
		"invalid":          {invalid, "invalid configuration file"},
		"broken include":   {brokenInclude, "was not included: included file not found"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := capture(t, func() error { return cmdAddConfigFile(tt.input) })
			if err == nil {
				t.Fatal("expected an error")
			}
			assertContains(t, err.Error(), tt.wantErr)
			if after, _ := os.ReadFile(main); string(after) != string(before) {
				t.Errorf("main configuration changed:\n%s", after)
			}
		})
	}

	setInput(t, "\n")
	out, _, err := capture(t, func() error { return cmdAddConfigFile("") })
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Cancelled.")
}

func TestRelativeLocalPaths(t *testing.T) {
	home := setHome(t)
	main := mainConfigPath(t)
	sharedDir := filepath.Join(t.TempDir(), "Dropbox", "zsync")
	shared := writeConfig(t, filepath.Join(sharedDir, "shared.toml"), `[[pairs]]
remote = "gdrive:/projects"
local = "../Projects"

[[pairs]]
remote = "gdrive:/here"
local = "."

[[pairs]]
remote = "gdrive:/docs"
local = "~/Documents"
`)
	writeConfig(t, main, `include = ["`+filepath.ToSlash(shared)+`"]

[[pairs]]
remote = "gdrive:/notes"
local = "notes"

[[pairs]]
remote = "gdrive:/abs"
local = "/abs/folder"
`)

	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, pair := range config.Pairs {
		got = append(got, pair.Local)
	}
	want := []string{
		filepath.Join(home, ".zsync", "notes"),
		filepath.Clean("/abs/folder"),
		filepath.Join(filepath.Dir(sharedDir), "Projects"),
		sharedDir,
		filepath.Join(home, "Documents"),
	}
	if runtime.GOOS == "windows" {
		// Without a drive letter, /abs/folder is relative on Windows.
		want[1] = filepath.Join(home, ".zsync", "abs", "folder")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestSyncAndRemoveWithRelativeLocalPath(t *testing.T) {
	log := setupEnv(t)
	sharedDir := t.TempDir()
	projects := filepath.Join(sharedDir, "Projects")
	os.Mkdir(projects, 0o755)
	os.Mkdir(filepath.Join(sharedDir, "Keep"), 0o755)
	writeFile(t, filepath.Join(projects, ignoreFile), "*.log\n")
	shared := writeConfig(t, filepath.Join(sharedDir, "shared.toml"), `[[pairs]]
remote = "gdrive:/keep"
local = "Keep"

[[pairs]]
remote = "gdrive:/projects"
local = "./Projects"
`)
	writeConfig(t, mainConfigPath(t), `include = ["`+filepath.ToSlash(shared)+`"]`)

	// rclone gets the resolved path, and .zsyncignore is found next to it.
	if _, _, err := capture(t, func() error { return cmdSync(up, false, false) }); err != nil {
		t.Fatal(err)
	}
	wantSync := "sync " + projects + " gdrive:/projects --exclude-from " + filepath.Join(projects, ignoreFile)
	if calls := syncCalls(t, log); len(calls) != 2 || calls[1] != wantSync {
		t.Errorf("unexpected rclone calls: %q", calls)
	}

	// Removing it from the included file works although the file has a relative path.
	setInput(t, "2\n")
	out, _, err := capture(t, cmdRemove)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "  2. "+projects+" <-> gdrive:/projects  (from "+shared+")")
	included, _ := readConfigFile(shared)
	if !reflect.DeepEqual(included.Pairs, []Pair{{Remote: "gdrive:/keep", Local: "Keep"}}) {
		t.Errorf("unexpected pairs left: %v", included.Pairs)
	}
}

func TestCmdAddDuplicateOfRelativeLocalPath(t *testing.T) {
	setupEnv(t)
	home, _ := os.UserHomeDir()
	docs := filepath.Join(home, "Documents")
	os.Mkdir(docs, 0o755)
	writeConfig(t, mainConfigPath(t), "[[pairs]]\nremote = \"gdrive:/docs\"\nlocal = \"~/Documents\"\n")
	resolvedHome, _ := filepath.EvalSymlinks(home)
	if resolvedHome != home {
		t.Skip("home folder path contains symlinks")
	}

	setInput(t, "gdrive:/docs\n"+docs+"\n")
	if _, _, err := capture(t, cmdAdd); err == nil || err.Error() != "this folder pair is already registered" {
		t.Errorf("expected a duplicate error, got %v", err)
	}
}
