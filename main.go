package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// ignoreFile is the per-folder file with rclone exclude patterns, passed to `rclone sync --exclude-from`.
const ignoreFile = ".zsyncignore"

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

type direction int

const (
	up direction = iota
	down
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "zsync",
		Short:         "Sync local and remote folder pairs using rclone",
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newSyncCmd("up", "Upload local folders to their remotes (local -> remote)", up),
		newSyncCmd("down", "Download remote folders to their local folders (remote -> local)", down),
		simpleCmd("add", "Add a new folder pair", cmdAdd),
		&cobra.Command{
			Use:   "add-config-file [file]",
			Short: "Include the pairs of another configuration file (asks for the file if not given)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return cmdAddConfigFile(strings.Join(args, ""))
			},
		},
		simpleCmd("remove", "Remove a registered folder pair", cmdRemove),
		simpleCmd("list", "List registered folder pairs", cmdList),
		simpleCmd("config", "Open the configuration file in your editor ($VISUAL or $EDITOR)", cmdConfig),
		simpleCmd("check", "Check that all registered local and remote folders exist", cmdCheck),
	)
	return root
}

func newSyncCmd(name, short string, dir direction) *cobra.Command {
	var progress, dryRun bool
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return cmdSync(dir, progress, dryRun)
		},
	}
	cmd.Flags().BoolVarP(&progress, "progress", "p", false, "Show transfer progress")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Simulate the sync without changing anything")
	return cmd
}

func simpleCmd(name, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return run() },
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func cmdSync(dir direction, progress, dryRun bool) error {
	if err := ensureRcloneInstalled(); err != nil {
		return err
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if len(config.Pairs) == 0 {
		fmt.Println("No folders registered. Use `zsync add` to add one.")
		return nil
	}

	// Nothing is synced unless every folder exists: a missing folder usually means a typo
	// or an unmounted drive, and syncing from an empty source would delete the destination.
	statuses, err := checkFolders(config.Pairs)
	if err != nil {
		return err
	}
	if missing := reportMissingFolders(config.Pairs, statuses); missing > 0 {
		return fmt.Errorf("%d folder(s) not found, nothing was synced", missing)
	}

	failures := 0
	for _, pair := range config.Pairs {
		source, dest := pair.Local, pair.Remote
		if dir == down {
			source, dest = pair.Remote, pair.Local
		}
		fmt.Printf("==> %s -> %s\n", source, dest)
		// The local folder's .zsyncignore (rclone exclude patterns) is used in both directions.
		excludeFrom := ""
		if path := filepath.Join(pair.Local, ignoreFile); isFile(path) {
			excludeFrom = path
			fmt.Printf("    using excludes from %s\n", path)
		}
		if err := rcloneSync(source, dest, excludeFrom, progress, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			failures++
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d of %d syncs failed", failures, len(config.Pairs))
	}
	fmt.Println("Sync complete.")
	return nil
}

func cmdAdd() error {
	if err := ensureRcloneInstalled(); err != nil {
		return err
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}

	var remote string
	for {
		input, err := prompt("Remote folder (e.g. gdrive:/my_folder): ")
		if err != nil {
			return err
		}
		if !strings.Contains(input, ":") {
			fmt.Fprintln(os.Stderr, "Invalid format: use <remote>:<path>, as configured in rclone.")
			continue
		}
		fmt.Println("Checking remote folder...")
		ok, err := remoteDirExists(input)
		if err != nil {
			return err
		}
		if ok {
			remote = input
			break
		}
		fmt.Fprintf(os.Stderr, "Remote folder not found: %s\n", input)
	}

	var local string
	for {
		input, err := prompt("Local folder (e.g. /home/user/my_folder): ")
		if err != nil {
			return err
		}
		path := expandTilde(input)
		if isDir(path) {
			local, err = canonicalize(path)
			if err != nil {
				return fmt.Errorf("failed to resolve %s: %w", path, err)
			}
			break
		}
		fmt.Fprintf(os.Stderr, "Local folder not found: %s\n", path)
	}

	pair := Pair{Remote: remote, Local: local}
	if slices.ContainsFunc(config.Pairs, pair.same) {
		return errors.New("this folder pair is already registered")
	}
	// New pairs always go to the main configuration file, never to included ones.
	path, err := configFile()
	if err != nil {
		return err
	}
	err = updateConfigFile(path, func(c *Config) error {
		c.Pairs = append(c.Pairs, pair)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("Added: %s <-> %s\n", pair.Local, pair.Remote)
	return nil
}

// newConfigFileTemplate is written to configuration files created by add-config-file.
const newConfigFileTemplate = `# Folder pairs for zsync, included from the main configuration file. Example:
#
# [[pairs]]
# remote = "gdrive:/my_folder"
# local = "/home/user/my_folder"
`

func cmdAddConfigFile(input string) error {
	mainPath, err := configFile()
	if err != nil {
		return err
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}

	if input == "" {
		input, err = prompt("Configuration file to include (e.g. ~/Dropbox/zsync.toml, empty to cancel): ")
		if err != nil {
			return err
		}
		if input == "" {
			fmt.Println("Cancelled.")
			return nil
		}
	}
	// Saved as an absolute path, since relative includes are relative to ~/.zsync, not to
	// the current folder.
	path, err := filepath.Abs(expandTilde(input))
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", input, err)
	}
	if path == mainPath {
		return errors.New("that is the main configuration file")
	}
	for _, include := range config.Include {
		if resolvePath(mainPath, include) == path {
			return fmt.Errorf("%s is already included", path)
		}
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		answer, err := prompt(fmt.Sprintf("%s does not exist. Create it? [y/N] ", path))
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Println("Cancelled.")
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(newConfigFileTemplate), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
		fmt.Printf("Created %s\n", path)
	case err != nil:
		return fmt.Errorf("failed to read %s: %w", path, err)
	case info.IsDir():
		return fmt.Errorf("%s is a folder, not a configuration file", path)
	}

	included, err := readConfigFile(path)
	if err != nil {
		return err
	}
	original, readErr := os.ReadFile(mainPath)
	err = updateConfigFile(mainPath, func(c *Config) error {
		c.Include = append(c.Include, path)
		return nil
	})
	if err != nil {
		return err
	}
	// The file's own includes are checked by loading everything; on error the main
	// configuration file is restored exactly as it was.
	if _, err := loadConfig(); err != nil {
		var undoErr error
		if readErr == nil {
			undoErr = os.WriteFile(mainPath, original, 0o644)
		} else {
			undoErr = os.Remove(mainPath)
		}
		if undoErr != nil {
			return fmt.Errorf("%w (and failed to undo the include: %v)", err, undoErr)
		}
		return fmt.Errorf("%s was not included: %w", path, err)
	}
	fmt.Printf("Included: %s (%d pair(s))\n", path, len(included.Pairs))
	return nil
}

func cmdRemove() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if len(config.Pairs) == 0 {
		fmt.Println("No folders registered.")
		return nil
	}
	printPairs(config)

	var index int
	for {
		input, err := prompt("Number of the pair to remove (empty to cancel): ")
		if err != nil {
			return err
		}
		if input == "" {
			fmt.Println("Cancelled.")
			return nil
		}
		n, err := strconv.Atoi(input)
		if err == nil && n >= 1 && n <= len(config.Pairs) {
			index = n - 1
			break
		}
		fmt.Fprintf(os.Stderr, "Enter a number between 1 and %d.\n", len(config.Pairs))
	}

	// The pair is removed from the file it was defined in, which may be an included one.
	removed := config.Pairs[index]
	err = updateConfigFile(removed.Source, func(c *Config) error {
		// The file may write the local folder as a relative or ~ path.
		i := slices.IndexFunc(c.Pairs, func(p Pair) bool {
			return p.Remote == removed.Remote && resolvePath(removed.Source, p.Local) == removed.Local
		})
		if i < 0 {
			return fmt.Errorf("pair not found in %s", removed.Source)
		}
		c.Pairs = slices.Delete(c.Pairs, i, i+1)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("Removed: %s <-> %s\n", removed.Local, removed.Remote)
	if main, _ := configFile(); removed.Source != main {
		fmt.Printf("    (from %s)\n", removed.Source)
	}
	return nil
}

func cmdList() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if len(config.Pairs) == 0 {
		fmt.Println("No folders registered. Use `zsync add` to add one.")
	} else {
		printPairs(config)
	}
	return nil
}

func cmdCheck() error {
	if err := ensureRcloneInstalled(); err != nil {
		return err
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if len(config.Pairs) == 0 {
		fmt.Println("No folders registered. Use `zsync add` to add one.")
		return nil
	}

	statuses, err := checkFolders(config.Pairs)
	if err != nil {
		return err
	}
	missing := 0
	main, _ := configFile()
	for i, pair := range config.Pairs {
		st := statuses[i]
		printPair(os.Stdout, i, pair, main)
		fmt.Printf("     local:  %s\n", status(st.localOK))
		fmt.Printf("     remote: %s\n", status(st.remoteOK))
		missing += st.missing()
	}

	if missing > 0 {
		return fmt.Errorf("%d folder(s) not found", missing)
	}
	fmt.Println("All folders exist.")
	return nil
}

func cmdConfig() error {
	path, err := configFile()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := (&Config{}).save(); err != nil {
			return err
		}
	}

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
		if runtime.GOOS == "windows" {
			editor = "notepad"
		}
	}
	parts := editorCommand(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("editor `%s` exited with %w", editor, err)
		}
		return fmt.Errorf("failed to start editor `%s` (set $VISUAL or $EDITOR): %w", editor, err)
	}

	if _, err := loadConfig(); err != nil {
		return fmt.Errorf("the configuration has errors, run `zsync config` again to fix them: %w", err)
	}
	return nil
}

// editorCommand splits the editor setting into program and arguments, e.g. "code --wait".
// A setting that is itself an existing file is kept whole, so paths with spaces such as
// `C:\Program Files\Notepad++\notepad++.exe` work without quoting.
func editorCommand(editor string) []string {
	if isFile(editor) {
		return []string{editor}
	}
	if program, args, ok := strings.Cut(editor, `"`); ok && strings.TrimSpace(program) == "" {
		// A quoted program path followed by arguments: "C:\Program Files\x.exe" --wait
		if program, rest, ok := strings.Cut(args, `"`); ok {
			return append([]string{program}, strings.Fields(rest)...)
		}
	}
	return strings.Fields(editor)
}

// folderStatus tells whether a pair's local and remote folders exist.
type folderStatus struct {
	localOK, remoteOK bool
}

func (s folderStatus) missing() int {
	n := 0
	if !s.localOK {
		n++
	}
	if !s.remoteOK {
		n++
	}
	return n
}

// checkFolders checks that the folders of every pair exist. The remote folders,
// which need a network round trip each, are checked in parallel.
func checkFolders(pairs []Pair) ([]folderStatus, error) {
	statuses := make([]folderStatus, len(pairs))
	errs := make([]error, len(pairs))
	var wg sync.WaitGroup
	for i, pair := range pairs {
		statuses[i].localOK = isDir(pair.Local)
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i].remoteOK, errs[i] = remoteDirExists(pair.Remote)
		}()
	}
	wg.Wait()
	return statuses, errors.Join(errs...)
}

// reportMissingFolders prints the pairs with missing folders to stderr and returns
// how many folders are missing.
func reportMissingFolders(pairs []Pair, statuses []folderStatus) int {
	missing := 0
	main, _ := configFile()
	for i, pair := range pairs {
		st := statuses[i]
		if st.missing() == 0 {
			continue
		}
		if missing == 0 {
			fmt.Fprintln(os.Stderr, "Some registered folders do not exist:")
		}
		missing += st.missing()
		printPair(os.Stderr, i, pair, main)
		if !st.localOK {
			fmt.Fprintf(os.Stderr, "     local folder not found:  %s\n", pair.Local)
		}
		if !st.remoteOK {
			fmt.Fprintf(os.Stderr, "     remote folder not found: %s\n", pair.Remote)
		}
	}
	return missing
}

func printPairs(config *Config) {
	main, _ := configFile()
	for i, pair := range config.Pairs {
		printPair(os.Stdout, i, pair, main)
	}
}

// printPair prints a numbered pair, noting the file it comes from when that is
// not the main configuration file.
func printPair(w io.Writer, i int, pair Pair, mainConfig string) {
	fmt.Fprintf(w, "%3d. %s <-> %s", i+1, pair.Local, pair.Remote)
	if pair.Source != mainConfig {
		fmt.Fprintf(w, "  (from %s)", pair.Source)
	}
	fmt.Fprintln(w)
}

func status(ok bool) string {
	if ok {
		return "ok"
	}
	return "NOT FOUND"
}

var stdin = bufio.NewReader(os.Stdin)

func prompt(message string) (string, error) {
	fmt.Print(message)
	line, err := stdin.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		if errors.Is(err, io.EOF) {
			return "", errors.New("input closed")
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func expandTilde(input string) string {
	// Accepts "~", "~/..." and, on Windows, "~\...".
	if rest, ok := strings.CutPrefix(input, "~"); ok && (rest == "" || os.IsPathSeparator(rest[0])) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return input
}

func canonicalize(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
