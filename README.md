# zsync

Sync pairs of local and remote folders (Google Drive, OneDrive, Dropbox... any
[rclone](https://rclone.org) remote) with a single command.

## Requirements

[rclone](https://rclone.org/install/) installed, with your remotes already set
up via `rclone config`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/master/install.sh | bash
```

Then open a new terminal. Windows binaries and other options are in the
[full guide](docs/GUIDE.md#installation-options).

## Usage

```sh
zsync add      # register a folder pair (asks for the remote and the local folder)
zsync list     # list registered pairs
zsync remove   # remove a pair
zsync up       # upload: local -> remote
zsync down     # download: remote -> local
zsync config   # edit the config file in your editor
```

Add `-p` to `up`/`down` to show progress, or `-n` to do a dry run.

> **Warning:** syncing makes the destination identical to the source, so files
> that exist only in the destination are **deleted**. When in doubt, use `-n`
> first.

To skip files, create a `.zsyncignore` file in the local folder using
[rclone filter rules](https://rclone.org/filtering/):

```text
- *.log
- node_modules/**
```

See the [full guide](docs/GUIDE.md) for details.
