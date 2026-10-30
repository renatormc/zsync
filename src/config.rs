use std::fs;
use std::path::PathBuf;

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

/// A synced folder pair: one local folder and one rclone remote folder.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Pair {
    pub remote: String,
    pub local: PathBuf,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Config {
    #[serde(default)]
    pub pairs: Vec<Pair>,
}

/// Hidden directory in the user's home where the configuration is stored (~/.zsync).
pub fn config_dir() -> Result<PathBuf> {
    let home = dirs::home_dir().context("could not determine the user's home directory")?;
    Ok(home.join(".zsync"))
}

pub fn config_file() -> Result<PathBuf> {
    Ok(config_dir()?.join("config.toml"))
}

impl Config {
    pub fn load() -> Result<Self> {
        let path = config_file()?;
        if !path.exists() {
            return Ok(Self::default());
        }
        let content = fs::read_to_string(&path)
            .with_context(|| format!("failed to read {}", path.display()))?;
        toml::from_str(&content).with_context(|| format!("invalid configuration file: {}", path.display()))
    }

    pub fn save(&self) -> Result<()> {
        let dir = config_dir()?;
        fs::create_dir_all(&dir).with_context(|| format!("failed to create {}", dir.display()))?;
        let path = config_file()?;
        let content = toml::to_string_pretty(self).context("failed to serialize configuration")?;
        fs::write(&path, content).with_context(|| format!("failed to write {}", path.display()))
    }
}
