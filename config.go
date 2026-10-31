package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Pair is a synced folder pair: one local folder and one rclone remote folder.
type Pair struct {
	Remote string `toml:"remote"`
	// Local may be relative to the folder of the configuration file that defines the
	// pair, or start with ~. Pairs returned by loadConfig hold the resolved absolute path.
	Local string `toml:"local"`
	// Source is the configuration file the pair was read from.
	Source string `toml:"-"`
}

// same reports whether both pairs sync the same folders, wherever they were defined.
func (p Pair) same(other Pair) bool {
	return p.Remote == other.Remote && p.Local == other.Local
}

// Config is the content of one configuration file. Include lists other
// configuration files whose pairs are added to this one's.
type Config struct {
	Include []string `toml:"include,omitempty"`
	Pairs   []Pair   `toml:"pairs"`
}

// configDir returns the hidden directory in the user's home where the configuration is stored (~/.zsync).
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine the user's home directory: %w", err)
	}
	return filepath.Join(home, ".zsync"), nil
}

func configFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// loadConfig loads the main configuration file and, recursively, the files it includes.
// The returned config holds the main file's includes and the pairs of all files,
// each with its Source set and its Local resolved to an absolute path.
func loadConfig() (*Config, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	main, err := readConfigFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}

	all := &Config{Include: main.Include}
	loaded := map[string]bool{path: true}
	var collect func(path string, config *Config) error
	collect = func(path string, config *Config) error {
		for _, pair := range config.Pairs {
			pair.Source = path
			pair.Local = resolvePath(path, pair.Local)
			all.Pairs = append(all.Pairs, pair)
		}
		for _, include := range config.Include {
			included := resolvePath(path, include)
			// Files included more than once (or in a cycle) are loaded only once.
			if loaded[included] {
				continue
			}
			loaded[included] = true
			config, err := readConfigFile(included)
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("included file not found: %s (included from %s)", included, path)
			}
			if err != nil {
				return err
			}
			if err := collect(included, config); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(path, main); err != nil {
		return nil, err
	}
	return all, nil
}

// resolvePath returns the absolute path of a path written in the configuration file
// from (an include entry or a local folder). Relative paths are relative to the
// folder of that file, and ~ is the user's home folder.
func resolvePath(from, entry string) string {
	path := expandTilde(entry)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(from), path)
	}
	return filepath.Clean(path)
}

// readConfigFile reads a single configuration file, without following its includes.
// A missing file returns an error matching os.ErrNotExist.
func readConfigFile(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var config Config
	if _, err := toml.Decode(string(content), &config); err != nil {
		return nil, fmt.Errorf("invalid configuration file: %s: %w", path, err)
	}
	for i, pair := range config.Pairs {
		if pair.Remote == "" || pair.Local == "" {
			return nil, fmt.Errorf("invalid configuration file: %s: pair %d must have both `remote` and `local`", path, i+1)
		}
	}
	for _, include := range config.Include {
		if include == "" {
			return nil, fmt.Errorf("invalid configuration file: %s: `include` has an empty path", path)
		}
	}
	return &config, nil
}

// updateConfigFile reads one configuration file (empty if missing), applies change and writes it back.
func updateConfigFile(path string, change func(*Config) error) error {
	config, err := readConfigFile(path)
	if errors.Is(err, os.ErrNotExist) {
		config = &Config{}
	} else if err != nil {
		return err
	}
	if err := change(config); err != nil {
		return err
	}
	return config.writeFile(path)
}

// save writes the config as the main configuration file.
func (c *Config) save() error {
	path, err := configFile()
	if err != nil {
		return err
	}
	return c.writeFile(path)
}

func (c *Config) writeFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("failed to serialize configuration: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}
