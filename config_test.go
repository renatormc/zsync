package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfigMissingFile(t *testing.T) {
	setHome(t)
	config := mustLoad(t)
	if len(config.Pairs) != 0 {
		t.Errorf("expected no pairs, got %v", config.Pairs)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	home := setHome(t)
	want := []Pair{
		{Remote: "gdrive:/docs", Local: "/home/user/docs"},
		{Remote: `onedrive:/with "quotes"`, Local: "/home/user/ação"},
		{Remote: "gdrive:/win", Local: `C:\Users\me\My Documents`},
	}
	saveConfig(t, want...)

	// Read back as written, without resolving paths (C:\... is relative on Linux).
	config, err := readConfigFile(filepath.Join(home, ".zsync", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Pairs, want) {
		t.Errorf("got %v, want %v", config.Pairs, want)
	}
}

func TestConfigFileFormat(t *testing.T) {
	home := setHome(t)
	saveConfig(t, Pair{Remote: "gdrive:/docs", Local: "/home/user/docs"})

	content, err := os.ReadFile(filepath.Join(home, ".zsync", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "[[pairs]]\nremote = \"gdrive:/docs\"\nlocal = \"/home/user/docs\"\n"
	if string(content) != want {
		t.Errorf("got:\n%s\nwant:\n%s", content, want)
	}
}

// Configuration files written by the Rust version must keep working.
func TestLoadConfigWrittenByRustVersion(t *testing.T) {
	home := setHome(t)
	os.MkdirAll(filepath.Join(home, ".zsync"), 0o755)
	writeFile(t, filepath.Join(home, ".zsync", "config.toml"), `[[pairs]]
remote = "gdrive:/a"
local = "/a"

[[pairs]]
remote = "gdrive:/b"
local = "/b"
`)
	want := []Pair{{Remote: "gdrive:/a", Local: "/a"}, {Remote: "gdrive:/b", Local: "/b"}}
	if got := mustLoad(t).Pairs; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := map[string]string{
		"invalid toml":   "[[pairs]\n",
		"missing local":  "[[pairs]]\nremote = \"gdrive:/a\"\n",
		"missing remote": "[[pairs]]\nlocal = \"/a\"\n",
		"wrong type":     "pairs = 3\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			home := setHome(t)
			os.MkdirAll(filepath.Join(home, ".zsync"), 0o755)
			writeFile(t, filepath.Join(home, ".zsync", "config.toml"), content)

			_, err := loadConfig()
			if err == nil {
				t.Fatal("expected an error")
			}
			assertContains(t, err.Error(), "invalid configuration file")
		})
	}
}
