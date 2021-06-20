mod dependency;
mod input;
mod mods_set;

use std::collections::HashSet;
use std::error::Error;
use std::path::PathBuf;
use structopt::StructOpt;

use crate::input::InputMod;
use crate::mods_set::ModsSet;

// TODO: Figure out why it's not coloring the help info.
#[derive(Debug, StructOpt)]
#[structopt(
    name = "fmm",
    about = "Enable, disable, download, update, create, and delete Factorio mods."
)]
struct App {
    /// Deduplicate zipped mod versions, leaving only the latest version
    #[structopt(long)]
    dedup: bool,
    /// The path to the mods directory
    // TODO: Make optional, introduce config file to specify default path
    #[structopt(short = "f", long)]
    dir: PathBuf,
    /// Disable all mods.
    #[structopt(short = "o", long)]
    disable_all: bool,
    /// A list of mods to disable. Format is `mod_name` or `mod_name@version`.
    #[structopt(short, long)]
    disable: Vec<InputMod>,
    /// Enable the latest versions of all mods.
    #[structopt(short = "l", long)]
    enable_all: bool,
    /// A list of mods to enable. Enabling a mod will enable all of its dependencies as well.
    /// Format is `mod_name` or `mod_name@version`.
    #[structopt(short, long)]
    enable: Vec<InputMod>,
    /// Ignore mod dependencies when enabling them.
    #[structopt(long)]
    ignore_dependencies: bool,
    /// Include the base mod when calling `disable-all`.
    #[structopt(long)]
    include_base_mod: bool,
    /// A list of mods to remove. If no version is provided, the latest version will be removed.
    #[structopt(short, long)]
    remove: Vec<InputMod>,
}

fn main() -> Result<(), Box<dyn Error>> {
    let app = App::from_args();

    let mut set = ModsSet::new(&app.dir)?;

    for mod_ident in app.remove.iter() {
        set.remove(mod_ident)?;
    }

    if app.dedup {
        set.dedup()?;
    }

    if app.disable_all {
        set.disable_all(app.include_base_mod);
    }

    if app.enable_all {
        set.enable_all();
    }

    for mod_ident in app.disable.iter() {
        set.disable(mod_ident)?;
    }

    if app.enable.len() > 0 {
        let enabled_names: HashSet<String> = HashSet::new();
        let to_enable = app.enable.clone();

        for mod_ident in to_enable.iter() {
            set.enable(mod_ident)?;
        }
    }

    set.write_mod_list()?;

    Ok(())
}
