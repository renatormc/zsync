# zsync

A simple CLI, written in Rust, to sync pairs of local and remote folders
(Google Drive, OneDrive, Dropbox, S3... any remote supported by
[rclone](https://rclone.org)) using `rclone sync`.

Register your `local folder <-> remote folder` pairs once, then run
`zsync up` to upload or `zsync down` to download all of them at once.

> **Warning:** `rclone sync` makes the destination **identical** to the source,
> which includes **deleting** files in the destination that don't exist in the
> source. When in doubt, run with `-n` (`--dry-run`) first to see what would
> happen.

## Requirements

- [rclone](https://rclone.org/install/) installed and on your `PATH`.
- Remotes already configured with `rclone config`. zsync does **not** manage
  rclone's configuration; it only uses the remotes you have already created.

Check your remotes with:

```sh
rclone listremotes
```

## Installation

### Quick install (Linux/macOS, recommended)

```sh
curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/main/install.sh | bash
```

This downloads the latest release binary for your system into `~/.local/bin`
(creating the folder if needed) and, if `~/.local/bin` isn't already referenced
in your `~/.bashrc`, adds it to your `PATH` there. Open a new terminal (or run
`source ~/.bashrc`) afterwards.

To install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/main/install.sh | ZSYNC_VERSION=v0.1.0 bash
```

Running the same command again updates zsync.

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

## Usage

### `zsync add` — register a folder pair

Asks for the remote folder and the local folder and checks that both exist
before saving:

```text
$ zsync add
Remote folder (e.g. gdrive:/my_folder): gdrive:/Documents
Checking remote folder...
Local folder (e.g. /home/user/my_folder): ~/Documents
Added: /home/user/Documents <-> gdrive:/Documents
```

The remote folder uses rclone's format, `<remote-name>:<path>`. The local folder
accepts `~` and is saved as an absolute path.

### `zsync list` — list registered pairs

```text
$ zsync list
  1. /home/user/Documents <-> gdrive:/Documents
  2. /home/user/Work <-> onedrive:/Work
```

### `zsync remove` — remove a pair

Shows the numbered list and removes the number you type (empty input cancels):

```text
$ zsync remove
  1. /home/user/Documents <-> gdrive:/Documents
  2. /home/user/Work <-> onedrive:/Work
Number of the pair to remove (empty to cancel): 2
Removed: /home/user/Work <-> onedrive:/Work
```

Removing a pair only unregisters it; no files are deleted.

### `zsync up` — upload local → remote

Runs `rclone sync <local> <remote>` for each registered pair.

```sh
zsync up        # sync
zsync up -p     # show transfer progress
zsync up -n     # dry run: show what would be done without changing anything
```

### `zsync down` — download remote → local

Runs `rclone sync <remote> <local>` for each registered pair.

```sh
zsync down
zsync down -p
zsync down -n
```

### `up` and `down` options

| Option             | Description                                          |
|--------------------|------------------------------------------------------|
| `-p`, `--progress` | Show progress (passes `--progress` to rclone)        |
| `-n`, `--dry-run`  | Simulate without changing anything (passes `--dry-run`) |

If syncing one pair fails, the others still run and the command exits with a
non-zero status.

## Configuration

Pairs are stored in `~/.zsync/config.toml`, which you can also edit by hand:

```toml
[[pairs]]
remote = "gdrive:/Documents"
local = "/home/user/Documents"

[[pairs]]
remote = "onedrive:/Work"
local = "/home/user/Work"
```

## Publishing a new release (maintainers)

The `.github/workflows/release.yml` workflow builds the binaries and attaches
them to the release automatically when a `v*` tag is pushed:

```sh
# bump the version in Cargo.toml, then:
git commit -am "v0.1.0"
git tag v0.1.0
git push origin main --tags
```
