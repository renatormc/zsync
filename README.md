# zsync

Sync pairs of local and remote folders (Google Drive, OneDrive, Dropbox... any
[rclone](https://rclone.org) remote) with a single command.

## Requirements

[rclone](https://rclone.org/install/) installed, with your remotes already set
up via `rclone config`.

## Install

### Linux / macOS

```sh
curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/master/install.sh | bash
```

Then open a new terminal.

### Windows

1. Install rclone (if you don't have it yet) and set up your remotes:

   ```powershell
   winget install Rclone.Rclone
   rclone config
   ```

2. Install zsync from PowerShell:

   ```powershell
   irm https://raw.githubusercontent.com/renatormc/zsync/master/install.ps1 | iex
   ```

   This puts `zsync.exe` in `%LOCALAPPDATA%\Programs\zsync` and adds that folder
   to your user `PATH`.

3. Open a new terminal and run `zsync --help`.

To install manually instead, download `zsync-<version>-x86_64-pc-windows-msvc.zip`
(or `aarch64-…` for ARM) from the
[Releases](https://github.com/renatormc/zsync/releases) page, extract it and put
`zsync.exe` in a folder on your `PATH`.

Other options are in the [full guide](docs/GUIDE.md#installation-options).

## Usage

```sh
zsync add      # register a folder pair (asks for the remote and the local folder)
zsync add-config-file <file>  # include the pairs of another config file
zsync list     # list registered pairs
zsync remove   # remove a pair
zsync up       # upload: local -> remote
zsync down     # download: remote -> local
zsync config   # edit the config file in your editor
zsync check    # check that all registered folders exist
```

Add `-p` to `up`/`down` to show progress, or `-n` to do a dry run.
Before syncing, `up`/`down` check that all registered folders exist; if any is
missing, nothing is synced and the missing folders are listed.

> **Warning:** syncing makes the destination identical to the source, so files
> that exist only in the destination are **deleted**. When in doubt, use `-n`
> first.

On Windows, local folders are regular paths like `C:\Users\me\Documents`
(`~\Documents` also works).

To skip files, create a `.zsyncignore` file in the local folder with one
[rclone pattern](https://rclone.org/filtering/) per line:

```text
*.log
node_modules/**
```

Pairs are saved in `~/.zsync/config.toml`, which can also include pairs from
other files (e.g. one shared between computers):

```toml
include = ["work.toml", "~/Dropbox/zsync/shared.toml"]
```

`zsync add-config-file ~/Dropbox/zsync/shared.toml` adds an entry to that list
(and offers to create the file if it doesn't exist yet).

In any config file, `local` can also be `~/...` or relative to that file's
folder (e.g. `local = "../Projects"`), so a shared file works on every computer.

See the [full guide](docs/GUIDE.md#including-other-configuration-files) for details.
