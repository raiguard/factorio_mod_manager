use std::cmp::min;
use std::fs::File;
use std::io::{Read, Write};

use anyhow::{anyhow, Result};
use console::style;
use indicatif::{ProgressBar, ProgressStyle};
use reqwest::blocking::Client;
use semver::Version;
use serde::Deserialize;
use thiserror::Error;

use crate::config::Config;
use crate::types::{ModEntry, ModIdent};

pub fn download_mod(client: &Client, config: &Config, mod_ident: &ModIdent) -> Result<()> {
    // Get authentication token and username
    let portal_auth = config
        .portal_auth
        .as_ref()
        .ok_or(DownloadModErr::NoPortalAuth)?;

    // Download the mod's information
    let mod_info: ModPortalResult = serde_json::from_str(
        &client
            .get(format!(
                "https://mods.factorio.com/api/mods/{}",
                mod_ident.name
            ))
            .send()?
            .text()?,
    )?;

    // Get the corresponding release
    let release = if let Some(version_req) = &mod_ident.version_req {
        mod_info
            .releases
            .iter()
            .rev()
            .find(|release| version_req.matches(&release.version))
    } else {
        mod_info.releases.last()
    }
    .ok_or(DownloadModErr::NoMatchingRelease)?;

    // Download the mod
    let mut res = client
        .get(format!("https://mods.factorio.com{}", release.download_url,))
        .query(&[
            ("username", &portal_auth.username),
            ("token", &portal_auth.token),
        ])
        .send()?;
    let total_size = res
        .content_length()
        .ok_or(DownloadModErr::NoMatchingRelease)?;

    let pb = ProgressBar::new(total_size);
    pb.set_style(ProgressStyle::default_bar()
    .template("{msg} {spinner:.green} [{elapsed_precise}] [{bar:.cyan/blue}] {bytes} / {total_bytes} ({bytes_per_sec}, {eta})")
    .progress_chars("=> "));
    pb.set_message(format!(
        "{} {} v{}",
        style("Downloading").cyan().bold(),
        mod_ident.name,
        release.version
    ));

    // TODO: Use temporary extension
    let mut path = config.mods_dir.clone();
    path.push(&release.file_name);
    let mut file = File::create(path)?;

    let mut downloaded: u64 = 0;

    let mut buf = vec![0; 8_096];

    while downloaded < total_size {
        // TODO: Handle interrupted error
        let bytes = res.read(&mut buf)?;
        file.write_all(&buf[0..bytes])?;
        // Update progress bar
        downloaded = min(downloaded + (bytes as u64), total_size);
        pb.set_position(downloaded);
    }

    pb.finish_and_clear();
    println!(
        "{} {} v{}",
        style("Downloaded").cyan().bold(),
        mod_ident.name,
        release.version
    );

    Ok(())
}

#[derive(Debug, Error)]
enum DownloadModErr {
    #[error("Could not get content length")]
    NoContentLength,
    #[error("No matching release was found on the mod portal")]
    NoMatchingRelease,
    #[error("Could not find mod portal authentication")]
    NoPortalAuth,
}

#[derive(Debug, Deserialize)]
struct ModPortalResult {
    // downloads_count: u32,
    name: String,
    // owner: String,
    releases: Vec<ModPortalRelease>,
    // summary: String,
    // title: String,
    // category: Option<ModPortalTag>,
}

#[derive(Debug, Deserialize)]
struct ModPortalRelease {
    download_url: String,
    file_name: String,
    sha1: String,
    version: Version,
}