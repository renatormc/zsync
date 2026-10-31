# zsync — Full guide

- [Installation options](#installation-options)
- [Commands in detail](#commands-in-detail)
- [Ignoring files (.zsyncignore)](#ignoring-files-zsyncignore)
- [Configuration file](#configuration-file)
  - [Including other configuration files](#including-other-configuration-files)
- [Publishing a new release (maintainers)](#publishing-a-new-release-maintainers)

## Installation options

### Install script

```sh
curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/master/install.sh | bash
```

The script detects your OS and CPU, downloads the matching binary from the
latest release into `~/.local/bin` (creating the folder if needed) and, if
`~/.local/bin` isn't already referenced in your `~/.bashrc`, adds it to your
`PATH` there. Running it again updates zsync.

To install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/master/install.sh | ZSYNC_VERSION=v0.1.0 bash
```

If you use a shell other than bash (e.g. zsh), make sure `~/.local/bin` is on
your `PATH` yourself.

### Install script (Windows)

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/renatormc/zsync/master/install.ps1 | iex
```

The script downloads the binary for your CPU (x86_64 or ARM64) from the latest
release into `%LOCALAPPDATA%\Programs\zsync` and adds that folder to your user
`PATH`. Open a new terminal afterwards. Running it again updates zsync.

To install a specific version:

```powershell
$env:ZSYNC_VERSION = 'v0.1.0'; irm https://raw.githubusercontent.com/renatormc/zsync/master/install.ps1 | iex
```

rclone can be installed with `winget install Rclone.Rclone`.

### Prebuilt binary (manual)

Download the archive for your system from the
[Releases](https://github.com/renatormc/zsync/releases) page:

| System                | File                                                 |
|-----------------------|------------------------------------------------------|
| Linux x86_64          | `zsync-<version>-x86_64-unknown-linux-musl.tar.gz`   |
| Linux ARM64           | `zsync-<version>-aarch64-unknown-linux-musl.tar.gz`  |
| macOS Intel           | `zsync-<version>-x86_64-apple-darwin.tar.gz`         |
| macOS Apple Silicon   | `zsync-<version>-aarch64-apple-darwin.tar.gz`        |
| Windows x86_64        | `zsync-<version>-x86_64-pc-windows-msvc.zip`         |
| Windows ARM64         | `zsync-<version>-aarch64-pc-windows-msvc.zip`        |

On Linux/macOS, extract it and put the binary in a folder on your `PATH`:

```sh
tar xzf zsync-v0.1.0-x86_64-unknown-linux-musl.tar.gz
mkdir -p ~/.local/bin
mv zsync-v0.1.0-x86_64-unknown-linux-musl/zsync ~/.local/bin/
zsync --version
```

The Linux binaries are statically linked and work on any distribution. On
Windows, extract the zip and put `zsync.exe` in a folder on your `PATH`.

### From source

With [Go](https://go.dev/dl/) installed:

```sh
go install github.com/renatormc/zsync@latest
```

Or by cloning the repository:

```sh
git clone https://github.com/renatormc/zsync
cd zsync
go build
# binary at ./zsync
```

## Commands in detail

### `zsync add`

Asks for the remote folder and the local folder and checks that both exist
before saving. If one of them is invalid, it asks again.

```text
$ zsync add
Remote folder (e.g. gdrive:/my_folder): gdrive:/Documents
Checking remote folder...
Local folder (e.g. /home/user/my_folder): ~/Documents
Added: /home/user/Documents <-> gdrive:/Documents
```

- The remote folder uses rclone's format, `<remote-name>:<path>`, with a remote
  you already configured via `rclone config`.
- The local folder accepts `~` (`~/Documents`, or `~\Documents` on Windows) and
  is saved as an absolute path.
- Registering the same pair twice is rejected.

### `zsync list`

```text
$ zsync list
  1. /home/user/Documents <-> gdrive:/Documents
  2. /home/user/Work <-> onedrive:/Work
```

### `zsync remove`

Shows the numbered list and removes the number you type (empty input cancels):

```text
$ zsync remove
  1. /home/user/Documents <-> gdrive:/Documents
  2. /home/user/Work <-> onedrive:/Work
Number of the pair to remove (empty to cancel): 2
Removed: /home/user/Work <-> onedrive:/Work
```

Removing a pair only unregisters it; no files are deleted.

### `zsync check`

Checks that every registered local and remote folder exists. Useful after
editing the config by hand, or before a sync:

```text
$ zsync check
  1. /home/user/Documents <-> gdrive:/Documents
     local:  ok
     remote: ok
  2. /home/user/Work <-> onedrive:/Work
     local:  ok
     remote: NOT FOUND
Error: 1 folder(s) not found
```

It exits with a non-zero status if any folder is missing, so it can be used in
scripts (e.g. `zsync check && zsync up`).

### `zsync up` and `zsync down`

- `zsync up` runs `rclone sync <local> <remote>` for each registered pair.
- `zsync down` runs `rclone sync <remote> <local>` for each registered pair.

| Option             | Description                                             |
|--------------------|---------------------------------------------------------|
| `-p`, `--progress` | Show progress (passes `--progress` to rclone)           |
| `-n`, `--dry-run`  | Simulate without changing anything (passes `--dry-run`) |

`rclone sync` makes the destination **identical** to the source, which includes
**deleting** files in the destination that don't exist in the source.

Before syncing anything, both commands silently check that every registered
local and remote folder exists (the same check as `zsync check`). If any is
missing, **nothing is synced**: zsync lists the missing folders and exits with
an error. This guards against typos, unmounted drives and folders that were
moved, since syncing from an empty or wrong source would delete files in the
destination.

```text
$ zsync up
Some registered folders do not exist:
  2. /media/usb/Photos <-> gdrive:/Photos
     local folder not found:  /media/usb/Photos
Error: 1 folder(s) not found, nothing was synced
```

Once that check passes, if syncing one pair fails, the others still run and the
command exits with a non-zero status.

## Ignoring files (.zsyncignore)

Put a `.zsyncignore` file at the root of a registered local folder to list
what should not be synced. It works like `.gitignore`, but uses
[rclone's pattern syntax](https://rclone.org/filtering/) and is passed to
`rclone sync --exclude-from`.

```text
# Comments start with #
# Exclude all .log files, anywhere
*.log
# Exclude a folder and everything inside it, anywhere
node_modules/**
# A leading / anchors to the root of the synced folder
/build/**
```

Rules:

- Each line is one pattern to exclude. Empty lines and lines starting with
  `#` or `;` are ignored.
- `*` matches within a folder name, `**` matches across folders.
- To exclude a folder, use `folder/**`.

Behavior:

- The `.zsyncignore` of the **local** folder is used for both `up` and `down`.
  The file itself is synced like any other file, so it also ends up on the
  remote.
- Ignored files are left untouched on both sides: they're never copied, and
  they're never deleted from the destination.
- Only the file at the root of each registered folder is read; `.zsyncignore`
  files in subfolders are ignored. Use patterns like `sub/folder/**` instead.
- Test your rules with `zsync up -n` before syncing for real.

## Configuration file

Pairs are stored in `~/.zsync/config.toml` (on Windows,
`%USERPROFILE%\.zsync\config.toml`). To edit it by hand, run:

```sh
zsync config
```

This opens the file in the editor set in `$VISUAL` or `$EDITOR` (falling back
to `vi`, or `notepad` on Windows) and checks it for errors when you close the
editor. The editor command may include arguments, e.g. `EDITOR="code --wait"`.
On Windows, a path with spaces works as is or in quotes, e.g.
`"C:\Program Files\Notepad++\notepad++.exe" -multiInst`.

The file looks like this:

```toml
[[pairs]]
remote = "gdrive:/Documents"
local = "/home/user/Documents"

[[pairs]]
remote = "onedrive:/Work"
local = "/home/user/Work"
```

On Windows, write local paths in single quotes, or double the backslashes inside
double quotes, as TOML requires:

```toml
[[pairs]]
remote = "gdrive:/Documents"
local = 'C:\Users\me\Documents'   # same as "C:\\Users\\me\\Documents"
```

### Relative local paths

`local` doesn't have to be absolute. A relative path is relative to the folder
of the configuration file it is written in, and `~` is your home folder:

```toml
[[pairs]]
remote = "gdrive:/Projects"
local = "../Projects"     # next to the folder that holds this file

[[pairs]]
remote = "gdrive:/Documents"
local = "~/Documents"     # /home/<you>/Documents, C:\Users\<you>\Documents...
```

This makes a configuration file portable: one kept in a synced folder (see
[including other configuration files](#including-other-configuration-files))
works on every computer, whatever the user name or where that folder lives.
`zsync add` still saves absolute paths; `zsync list` and the sync commands show
the resolved ones.

zsync never touches rclone's own configuration.

### Including other configuration files

`config.toml` can pull in pairs from other TOML files with an `include` list.
This is handy to keep a set of pairs in a file shared between computers (e.g.
inside a synced folder), or to split work and personal folders:

```toml
include = [
  "work.toml",                        # relative to ~/.zsync
  "~/Dropbox/zsync/shared.toml",      # ~ is your home folder
  "/etc/zsync/common.toml",           # absolute path
]

[[pairs]]
remote = "gdrive:/Documents"
local = "/home/user/Documents"
```

The easiest way to add one is:

```sh
zsync add-config-file ~/Dropbox/zsync/shared.toml   # or without the path, to be asked for it
```

It saves the file's absolute path in `include`, after checking that it is a
valid configuration file that isn't already included. If the file doesn't exist,
it offers to create it with a commented example, ready for you to fill in on
this or another computer. You can also edit the list by hand with `zsync config`;
`include` must come before the first `[[pairs]]`, as TOML requires. Included
files have the same format: their own `[[pairs]]` and, optionally, their own
`include` list.

- Relative paths, in `include` and in `local`, are relative to the folder of
  the file that contains them.
- A file included more than once (or in a loop) is loaded only once.
- A missing or invalid included file is an error, so nothing syncs until it's
  fixed; `zsync config` checks the included files too.
- `zsync list`, `remove` and `check` show `(from <file>)` next to pairs that
  come from an included file.
- `zsync add` always saves new pairs to `config.toml`. `zsync remove` removes
  the pair from the file it is defined in, even if that is an included file.

## Publishing a new release (maintainers)

The `.github/workflows/release.yml` workflow builds the binaries for all
platforms and attaches them to a GitHub release when a `v*` tag is pushed:

```sh
# the version shown by `zsync --version` comes from the tag
git tag v0.1.1
git push origin master --tags
```
