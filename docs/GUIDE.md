# zsync — Full guide

- [Installation options](#installation-options)
- [Commands in detail](#commands-in-detail)
- [Ignoring files (.zsyncignore)](#ignoring-files-zsyncignore)
- [Configuration file](#configuration-file)
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

### Prebuilt binary (manual)

Download the archive for your system from the
[Releases](https://github.com/renatormc/zsync/releases) page:

| System                | File                                                 |
|-----------------------|------------------------------------------------------|
| Linux x86_64          | `zsync-<version>-x86_64-unknown-linux-musl.tar.gz`   |
| Linux ARM64           | `zsync-<version>-aarch64-unknown-linux-musl.tar.gz`  |
| macOS Intel           | `zsync-<version>-x86_64-apple-darwin.tar.gz`         |
| macOS Apple Silicon   | `zsync-<version>-aarch64-apple-darwin.tar.gz`        |
| Windows               | `zsync-<version>-x86_64-pc-windows-msvc.zip`         |

On Linux/macOS, extract it and put the binary in a folder on your `PATH`:

```sh
tar xzf zsync-v0.1.0-x86_64-unknown-linux-musl.tar.gz
mkdir -p ~/.local/bin
mv zsync-v0.1.0-x86_64-unknown-linux-musl/zsync ~/.local/bin/
zsync --version
```

The Linux binaries are statically linked (musl) and work on any distribution.

### From source

With [Rust](https://rustup.rs) installed:

```sh
cargo install --git https://github.com/renatormc/zsync
```

Or by cloning the repository:

```sh
git clone https://github.com/renatormc/zsync
cd zsync
cargo build --release
# binary at target/release/zsync
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
- The local folder accepts `~` and is saved as an absolute path.
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

### `zsync up` and `zsync down`

- `zsync up` runs `rclone sync <local> <remote>` for each registered pair.
- `zsync down` runs `rclone sync <remote> <local>` for each registered pair.

| Option             | Description                                             |
|--------------------|---------------------------------------------------------|
| `-p`, `--progress` | Show progress (passes `--progress` to rclone)           |
| `-n`, `--dry-run`  | Simulate without changing anything (passes `--dry-run`) |

`rclone sync` makes the destination **identical** to the source, which includes
**deleting** files in the destination that don't exist in the source.

If syncing one pair fails, the others still run and the command exits with a
non-zero status.

## Ignoring files (.zsyncignore)

Put a `.zsyncignore` file at the root of a registered local folder to control
what gets synced. It works like `.gitignore`, but uses
[rclone's filter format](https://rclone.org/filtering/) and is passed to
`rclone sync --filter-from`.

```text
# Comments start with #
# Keep this one file even though *.log is excluded below (first match wins)
+ keep.log
# Exclude all .log files, anywhere
- *.log
# Exclude a folder and everything inside it, anywhere
- node_modules/**
# A leading / anchors to the root of the synced folder
- /build/**
```

Rules:

- Every rule must start with `- ` (exclude) or `+ ` (include). A line without
  a prefix (e.g. `build/`) is an error and the sync for that folder fails.
- Rules are checked from top to bottom and the **first match wins**.
- `*` matches within a folder name, `**` matches across folders.

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

Pairs are stored in `~/.zsync/config.toml`. To edit it by hand, run:

```sh
zsync config
```

This opens the file in the editor set in `$VISUAL` or `$EDITOR` (falling back
to `vi`, or `notepad` on Windows) and checks it for errors when you close the
editor. The editor command may include arguments, e.g. `EDITOR="code --wait"`.

The file looks like this:

```toml
[[pairs]]
remote = "gdrive:/Documents"
local = "/home/user/Documents"

[[pairs]]
remote = "onedrive:/Work"
local = "/home/user/Work"
```

zsync never touches rclone's own configuration.

## Publishing a new release (maintainers)

The `.github/workflows/release.yml` workflow builds the binaries for all
platforms and attaches them to a GitHub release when a `v*` tag is pushed:

```sh
# bump the version in Cargo.toml and run `cargo build` to update Cargo.lock, then:
git commit -am "Release v0.1.1"
git tag v0.1.1
git push origin master --tags
```
