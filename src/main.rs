mod config;
mod rclone;

use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::process::ExitCode;

use anyhow::{Context, Result, bail};
use clap::{Parser, Subcommand};

use config::{Config, Pair};

#[derive(Parser)]
#[command(name = "zsync", version, about = "Sync local and remote folder pairs using rclone")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Upload local folders to their remotes (local -> remote)
    Up {
        /// Show transfer progress
        #[arg(short, long)]
        progress: bool,
        /// Simulate the sync without changing anything
        #[arg(short = 'n', long)]
        dry_run: bool,
    },
    /// Download remote folders to their local folders (remote -> local)
    Down {
        /// Show transfer progress
        #[arg(short, long)]
        progress: bool,
        /// Simulate the sync without changing anything
        #[arg(short = 'n', long)]
        dry_run: bool,
    },
    /// Add a new folder pair
    Add,
    /// Remove a registered folder pair
    Remove,
    /// List registered folder pairs
    List,
}

#[derive(Clone, Copy)]
enum Direction {
    Up,
    Down,
}

fn main() -> ExitCode {
    let cli = Cli::parse();
    let result = match cli.command {
        Commands::Up { progress, dry_run } => cmd_sync(Direction::Up, progress, dry_run),
        Commands::Down { progress, dry_run } => cmd_sync(Direction::Down, progress, dry_run),
        Commands::Add => cmd_add(),
        Commands::Remove => cmd_remove(),
        Commands::List => cmd_list(),
    };
    match result {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("Error: {e:#}");
            ExitCode::FAILURE
        }
    }
}

fn cmd_sync(direction: Direction, progress: bool, dry_run: bool) -> Result<()> {
    rclone::ensure_installed()?;
    let config = Config::load()?;
    if config.pairs.is_empty() {
        println!("No folders registered. Use `zsync add` to add one.");
        return Ok(());
    }

    let mut failures = 0;
    for pair in &config.pairs {
        let local = rclone::path_str(&pair.local)?;
        let (source, dest) = match direction {
            Direction::Up => (local, pair.remote.as_str()),
            Direction::Down => (pair.remote.as_str(), local),
        };
        println!("==> {source} -> {dest}");
        if let Err(e) = rclone::sync(source, dest, progress, dry_run) {
            eprintln!("Error: {e:#}");
            failures += 1;
        }
    }

    if failures > 0 {
        bail!("{failures} of {} syncs failed", config.pairs.len());
    }
    println!("Sync complete.");
    Ok(())
}

fn cmd_add() -> Result<()> {
    rclone::ensure_installed()?;
    let mut config = Config::load()?;

    let remote = loop {
        let input = prompt("Remote folder (e.g. gdrive:/my_folder): ")?;
        if !input.contains(':') {
            eprintln!("Invalid format: use <remote>:<path>, as configured in rclone.");
            continue;
        }
        println!("Checking remote folder...");
        if rclone::remote_dir_exists(&input)? {
            break input;
        }
        eprintln!("Remote folder not found: {input}");
    };

    let local = loop {
        let input = prompt("Local folder (e.g. /home/user/my_folder): ")?;
        let path = expand_tilde(&input);
        if path.is_dir() {
            break path
                .canonicalize()
                .with_context(|| format!("failed to resolve {}", path.display()))?;
        }
        eprintln!("Local folder not found: {}", path.display());
    };

    let pair = Pair { remote, local };
    if config.pairs.contains(&pair) {
        bail!("this folder pair is already registered");
    }
    println!("Added: {} <-> {}", pair.local.display(), pair.remote);
    config.pairs.push(pair);
    config.save()
}

fn cmd_remove() -> Result<()> {
    let mut config = Config::load()?;
    if config.pairs.is_empty() {
        println!("No folders registered.");
        return Ok(());
    }
    print_pairs(&config);

    let index = loop {
        let input = prompt("Number of the pair to remove (empty to cancel): ")?;
        if input.is_empty() {
            println!("Cancelled.");
            return Ok(());
        }
        match input.parse::<usize>() {
            Ok(n) if (1..=config.pairs.len()).contains(&n) => break n - 1,
            _ => eprintln!("Enter a number between 1 and {}.", config.pairs.len()),
        }
    };

    let removed = config.pairs.remove(index);
    config.save()?;
    println!("Removed: {} <-> {}", removed.local.display(), removed.remote);
    Ok(())
}

fn cmd_list() -> Result<()> {
    let config = Config::load()?;
    if config.pairs.is_empty() {
        println!("No folders registered. Use `zsync add` to add one.");
    } else {
        print_pairs(&config);
    }
    Ok(())
}

fn print_pairs(config: &Config) {
    for (i, pair) in config.pairs.iter().enumerate() {
        println!("{:>3}. {} <-> {}", i + 1, pair.local.display(), pair.remote);
    }
}

fn prompt(message: &str) -> Result<String> {
    print!("{message}");
    io::stdout().flush()?;
    let mut line = String::new();
    if io::stdin().read_line(&mut line)? == 0 {
        bail!("input closed");
    }
    Ok(line.trim().to_string())
}

fn expand_tilde(input: &str) -> PathBuf {
    if let Some(rest) = input.strip_prefix("~")
        && (rest.is_empty() || rest.starts_with('/'))
        && let Some(home) = dirs::home_dir()
    {
        return home.join(rest.trim_start_matches('/'));
    }
    Path::new(input).to_path_buf()
}
