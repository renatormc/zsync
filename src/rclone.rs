use std::path::Path;
use std::process::{Command, Stdio};

use anyhow::{Context, Result, bail};

/// Ensures the rclone executable is available on the PATH.
pub fn ensure_installed() -> Result<()> {
    let status = Command::new("rclone")
        .arg("version")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    match status {
        Ok(s) if s.success() => Ok(()),
        _ => bail!("rclone not found. Install it from https://rclone.org/install/ and set up your remotes with `rclone config`"),
    }
}

/// Checks whether the remote path exists and is a directory.
pub fn remote_dir_exists(remote: &str) -> Result<bool> {
    let output = Command::new("rclone")
        .args(["lsjson", "--stat", remote])
        .output()
        .context("failed to run rclone")?;
    if !output.status.success() {
        return Ok(false);
    }
    let stdout = String::from_utf8_lossy(&output.stdout);
    // The root of a remote (e.g. "gdrive:") may return nothing; success is enough in that case.
    let compact: String = stdout.split_whitespace().collect();
    Ok(compact.is_empty() || compact.contains("\"IsDir\":true"))
}

/// Runs `rclone sync <source> <dest>`.
pub fn sync(source: &str, dest: &str, exclude_from: Option<&Path>, progress: bool, dry_run: bool) -> Result<()> {
    let mut cmd = Command::new("rclone");
    cmd.args(["sync", source, dest]);
    if let Some(path) = exclude_from {
        cmd.arg("--exclude-from").arg(path);
    }
    if progress {
        cmd.arg("--progress");
    }
    if dry_run {
        cmd.arg("--dry-run");
    }
    let status = cmd.status().context("failed to run rclone")?;
    if !status.success() {
        bail!("rclone sync {source} -> {dest} failed ({status})");
    }
    Ok(())
}

pub fn path_str(path: &Path) -> Result<&str> {
    path.to_str()
        .with_context(|| format!("path contains invalid characters: {}", path.display()))
}
