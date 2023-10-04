package fmm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// Manager manages mdos for a given game directory. A game directory is
// considered valid if it has either a config-path.cfg file or a
// config/config.ini file.
type Manager struct {
	DoSave bool
	Portal ModPortal

	gamePath         string
	internalModsPath string
	modListJsonPath  string
	modsPath         string

	mods map[string]*Mod
}

type PlayerData struct {
	Token    string
	Username string
}

// Creates a new Manager for the given game directory. A game directory is
// considered valid if it has either a config-path.cfg file or a
// config/config.ini file. The player's username and token will
// be automatically retrieved from `player-data.json` if it exists.
func NewManager(gamePath string) (*Manager, error) {
	if !isValidGameDir(gamePath) {
		return nil, ErrInvalidGameDirectory
	}

	// TODO: Handle config-path.cfg and config.ini path definitions

	m := Manager{
		DoSave: true,
		Portal: ModPortal{
			baseVersion:  [4]uint16{},
			downloadPath: filepath.Join(gamePath, "mods"),
			mods:         map[string]*PortalModInfo{},
			server:       "https://mods.factorio.com",
		},

		gamePath:         gamePath,
		internalModsPath: filepath.Join(gamePath, "data"),
		modListJsonPath:  filepath.Join(gamePath, "mods", "mod-list.json"),
		modsPath:         filepath.Join(gamePath, "mods"),
		mods:             map[string]*Mod{},
	}

	if err := m.readPlayerData(); err != nil {
		return nil, errors.Join(errors.New("unable to get player data"), err)
	}

	if !entryExists(m.modsPath) {
		if err := os.Mkdir("mods", 0755); err != nil {
			return nil, errors.Join(errors.New("failed to create mods directory"), err)
		}
	}

	if err := m.parseInternalMods(); err != nil {
		return nil, errors.Join(errors.New("error parsing internal mods"), err)
	}

	if err := m.parseMods(); err != nil {
		return nil, errors.Join(errors.New("error parsing mods"), err)
	}

	for _, mod := range m.mods {
		slices.SortFunc(mod.releases, func(a *Release, b *Release) int {
			switch a.Version.Cmp(&b.Version) {
			case VersionLt:
				return -1
			case VersionGt:
				return 1
			case VersionEq:
				return 0
			// Should be unreachable
			default:
				return 0
			}
		})
	}

	if err := m.parseModList(); err != nil {
		return nil, errors.Join(errors.New("error parsing mod-list.json"), err)
	}

	return &m, nil
}

// Requests the mod to be disabled.
func (m *Manager) Disable(modName string) error {
	mod, err := m.GetMod(modName)
	if err != nil {
		return err
	}
	if mod.Enabled == nil {
		return &errModAlreadyDisabled{ModIdent: ModIdent{Name: modName}}
	}
	mod.Enabled = nil
	return nil
}

// TODO: Handle config-path.cfg and config.ini path definitions

// Requests all non-internal mods to be disabled.
func (m *Manager) DisableAll() {
	for _, mod := range m.mods {
		// base is the only mod that is always enabled by default
		if mod.Name != "base" {
			mod.Enabled = nil
		}
	}
}

// Enable the given mod, if it exists. If version is nil, it will default to
// the newest local release. Returns the version that was enabled, if any.
func (m *Manager) Enable(ident ModIdent) (*Version, error) {
	mod, err := m.GetMod(ident.Name)
	if err != nil {
		return nil, err
	}
	release := mod.GetRelease(ident.Version)
	if release == nil {
		return nil, errors.New("unable to find a matching release")
	}
	if mod.Enabled != nil && *mod.Enabled == release.Version {
		return nil, nil
	}
	toEnable := &release.Version
	mod.Enabled = toEnable
	return toEnable, nil
}

// Retrieves the corresponding Mod object.
func (m *Manager) GetMod(name string) (*Mod, error) {
	mod := m.mods[name]
	if mod == nil {
		return nil, errors.New("mod not found")
	}
	return mod, nil
}

// Applies the requested modifications and saves to mod-list.json.
func (m *Manager) Save() error {
	if !m.DoSave {
		return nil
	}
	var ModListJson modListJson
	for name, mod := range m.mods {
		ModListJson.Mods = append(ModListJson.Mods, modListJsonMod{
			Name:       name,
			Enabled:    mod.Enabled != nil,
			Version:    mod.Enabled,
			isInternal: mod.isInternal,
		})
	}
	sort.Sort(ModListJson.Mods)
	marshaled, err := json.MarshalIndent(ModListJson, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.modListJsonPath, marshaled, 0666)
}

// Returns the current upload API key.
func (m *Manager) GetApiKey() string {
	return m.Portal.apiKey
}

// Returns true if the Manager has an upload API key.
func (m *Manager) HasApiKey() bool {
	return m.Portal.apiKey != ""
}

// Sets the API key used for mod uploading.
func (m *Manager) SetApiKey(key string) {
	m.Portal.apiKey = key
}

// Returns the current player data.
func (m *Manager) GetPlayerData() PlayerData {
	return m.Portal.playerData
}

// Returns true if the Manager has valid player data.
func (m *Manager) HasPlayerData() bool {
	return m.Portal.playerData.Token != "" && m.Portal.playerData.Username != ""
}

// Sets the player data used for downloading mods. The player data will be
// automatically retrieved from the game directory if it is available.
func (m *Manager) SetPlayerData(playerData PlayerData) {
	m.Portal.playerData = playerData
}

func (m *Manager) addRelease(release *Release, isInternal bool) {
	mod := m.mods[release.Name]
	if mod == nil {
		mod = &Mod{
			Name:       release.Name,
			releases:   []*Release{},
			isInternal: isInternal,
		}
		m.mods[release.Name] = mod
	}
	mod.releases = append(mod.releases, release)

}

func (m *Manager) parseModList() error {
	modListJsonData, err := os.ReadFile(m.modListJsonPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.Enable(ModIdent{Name: "base"})
			return nil
		}
		return errors.Join(errors.New("error reading mod-list.json"), err)
	}

	var modListJson modListJson
	if err = json.Unmarshal(modListJsonData, &modListJson); err != nil {
		return errors.Join(errors.New("error parsing mod-list.json"), err)
	}
	for _, modEntry := range modListJson.Mods {
		if !modEntry.Enabled {
			continue
		}
		mod := m.mods[modEntry.Name]
		if mod == nil {
			continue
		}
		if release := mod.GetRelease(modEntry.Version); release != nil {
			enabled := release.Version
			mod.Enabled = &enabled
		}
	}
	return nil
}

func (m *Manager) parseInternalMods() error {
	entries, err := os.ReadDir(m.internalModsPath)
	if err != nil {
		return errors.Join(errors.New("could not read internal mods directory"), err)
	}

	for _, entry := range entries {
		filename := entry.Name()
		if filename == "core" || !entry.Type().IsDir() {
			continue
		}
		// Not all directories are necessarily mods
		if _, err := os.Stat(filepath.Join(m.internalModsPath, filename, "info.json")); err != nil {
			continue
		}
		release, err := releaseFromFile(filepath.Join(m.internalModsPath, filename))
		if err != nil {
			return errors.Join(errors.New(fmt.Sprint("unable to parse ", filename)), err)
		}
		m.addRelease(release, true)
	}

	return nil
}

func (m *Manager) parseMods() error {
	entries, err := os.ReadDir(m.modsPath)
	if err != nil {
		return errors.Join(errors.New("could not read mods directory"), err)
	}

	for _, entry := range entries {
		filename := entry.Name()
		if filename == "mod-list.json" || filename == "mod-settings.dat" {
			continue
		}
		release, err := releaseFromFile(filepath.Join(m.modsPath, filename))
		if err != nil {
			return errors.Join(errors.New(fmt.Sprint("invalid mod ", filename)), err)
		}
		m.addRelease(release, false)
	}

	return nil
}

func (m *Manager) readPlayerData() error {
	playerDataJsonPath := filepath.Join(m.gamePath, "player-data.json")
	if !entryExists(playerDataJsonPath) {
		return nil
	}

	data, err := os.ReadFile(playerDataJsonPath)
	if err != nil {
		return errors.Join(errors.New("unable to read player-data.json"), err)
	}
	var playerDataJson playerDataJson
	err = json.Unmarshal(data, &playerDataJson)
	if err != nil {
		return errors.Join(errors.New("invalid player-data.json format"), err)
	}
	if playerDataJson.ServiceToken != nil {
		m.Portal.playerData.Token = *playerDataJson.ServiceToken
	}
	if playerDataJson.ServiceUsername != nil {
		m.Portal.playerData.Username = *playerDataJson.ServiceUsername
	}

	return nil
}

func entryExists(pathParts ...string) bool {
	_, err := os.Stat(filepath.Join(pathParts...))
	return err == nil
}

func isValidGameDir(dir string) bool {
	return entryExists(dir, "data", "base", "info.json")
}

// func expandDependencies(manager *Manager, mods []ModIdent) []ModIdent {
// 	visited := make(map[string]bool)
// 	toVisit := []Dependency{}
// 	for _, mod := range mods {
// 		toVisit = append(toVisit, Dependency{mod, DependencyRequired, VersionEq})
// 	}
// 	output := []ModIdent{}

// 	for i := 0; i < len(toVisit); i += 1 {
// 		mod := toVisit[i]
// 		if _, exists := visited[mod.Ident.Name]; exists {
// 			continue
// 		}
// 		visited[mod.Ident.Name] = true
// 		var ident ModIdent
// 		var deps []Dependency
// 		var err error
// 		if file := manager.Find(mod); file != nil {
// 			ident = file.Ident
// 			deps, err = file.Dependencies()
// 		} else if mod.Ident.Name == "base" {
// 			// TODO: Check against dependency constraint?
// 			ident = mod.Ident
// 		} else {
// 			var release *PortalModRelease
// 			release, err = portalGetRelease(mod)
// 			if err == nil {
// 				ident = ModIdent{mod.Ident.Name, &release.Version}
// 				deps = release.InfoJson.Dependencies
// 			}
// 		}
// 		if err != nil {
// 			errorln(err)
// 			continue
// 		}
// 		output = append(output, ident)
// 		for _, dep := range deps {
// 			if dep.Ident.Name == "base" {
// 				continue
// 			}
// 			if dep.Kind == DependencyRequired || dep.Kind == DependencyNoLoadOrder {
// 				toVisit = append(toVisit, dep)
// 			}
// 		}
// 	}

// 	return output
// }
