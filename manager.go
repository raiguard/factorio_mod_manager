package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

var internalMods = map[string]bool{
	"base": true,
	"core": true,
}

type mods map[string]*Mod

type Manager struct {
	DoSave bool

	apiKey           string
	downloadToken    string
	downloadUsername string
	gamePath         string
	modListJsonPath  string
	mods             mods
	modsPath         string
}

func NewManager(gamePath string) (*Manager, error) {
	if !isFactorioDir(gamePath) {
		return nil, errors.New("Invalid Factorio data directory")
	}

	m := Manager{
		DoSave:          true,
		gamePath:        gamePath,
		modListJsonPath: path.Join(gamePath, "mods", "mod-list.json"),
		mods:            mods{},
		modsPath:        path.Join(gamePath, "mods"),
	}

	if err := m.getPlayerData(); err != nil {
		return nil, errors.Join(errors.New("Unable to get player data"), err)
	}

	modListJsonPath := path.Join(m.modsPath, "mod-list.json")
	if !entryExists(m.modsPath) {
		if err := os.Mkdir("mods", 0755); err != nil {
			return nil, errors.Join(errors.New("Failed to create mods directory"), err)
		}
		// TODO: Auto-create mod-list.json
		// if err := os.WriteFile(modListJsonPath, ; err != nil {
		// 	return m, errors.Join(errors.New("Failed to create mod-list.json"), err)
		// }
	}

	if err := m.parseMods(); err != nil {
		return nil, errors.Join(errors.New("Error parsing mods"), err)
	}

	modListJsonData, err := os.ReadFile(modListJsonPath)
	if err != nil {
		return nil, errors.Join(errors.New("Error reading mod-list.json"), err)
	}
	var modListJson modListJson
	if err = json.Unmarshal(modListJsonData, &modListJson); err != nil {
		return nil, errors.Join(errors.New("Error parsing mod-list.json"), err)
	}
	for _, modEntry := range modListJson.Mods {
		if !modEntry.Enabled {
			continue
		}
		mod := m.mods[modEntry.Name]
		if mod == nil {
			continue
		}
		if release := mod.getRelease(modEntry.Version); release != nil {
			enabled := release.Version
			mod.Enabled = &enabled
		}
	}

	return &m, nil
}

func (m *Manager) Disable(modName string) error {
	mod, err := m.getMod(modName)
	if err != nil {
		return err
	}
	if mod.Enabled == nil {
		return errors.New("Mod is already disabled")
	}
	mod.Enabled = nil
	return nil
}

func (m *Manager) DisableAll() {
	for _, mod := range m.mods {
		if !internalMods[mod.Name] {
			mod.Enabled = nil
		}
	}
}

func (m *Manager) Enable(name string, version *Version) error {
	mod, err := m.getMod(name)
	if err != nil {
		return err
	}
	release := mod.getRelease(version)
	if release == nil {
		return errors.New("Unable to find a matching release")
	}
	enabled := release.Version
	mod.Enabled = &enabled
	return nil
}

func (m *Manager) getMod(name string) (*Mod, error) {
	mod := m.mods[name]
	if mod == nil {
		return nil, errors.New("Mod not found")
	}
	return mod, nil
}

func (m *Manager) Save() error {
	if !m.DoSave {
		return nil
	}
	var ModListJson modListJson
	for name, mod := range m.mods {
		ModListJson.Mods = append(ModListJson.Mods, modListJsonMod{
			Name:    name,
			Enabled: mod.Enabled != nil,
			Version: mod.Enabled,
		})
	}
	sort.Sort(ModListJson.Mods)
	marshaled, err := json.MarshalIndent(ModListJson, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.modListJsonPath, marshaled, fs.ModeExclusive)
}

func (m *Manager) getPlayerData() error {
	playerDataJsonPath := path.Join(m.gamePath, "player-data.json")
	if !entryExists(playerDataJsonPath) {
		return nil
	}

	data, err := os.ReadFile(playerDataJsonPath)
	if err != nil {
		return errors.Join(errors.New("Unable to read player-data.json"), err)
	}
	var playerDataJson playerDataJson
	err = json.Unmarshal(data, &playerDataJson)
	if err != nil {
		return errors.Join(errors.New("Invalid player-data.json format"), err)
	}
	if playerDataJson.ServiceToken != nil {
		m.downloadToken = *playerDataJson.ServiceToken
	}
	if playerDataJson.ServiceUsername != nil {
		m.downloadUsername = *playerDataJson.ServiceUsername
	}

	return nil
}

func (m *Manager) parseMods() error {
	entries, err := os.ReadDir(m.modsPath)
	if err != nil {
		return errors.Join(errors.New("Could not read mods directory"), err)
	}

	for _, entry := range entries {
		filename := entry.Name()
		if filename == "mod-list.json" || filename == "mod-settings.dat" {
			continue
		}
		release, err := releaseFromFile(filepath.Join(m.modsPath, filename))
		if err != nil {
			return errors.Join(errors.New(fmt.Sprint("Unable to parse ", filename)), err)
		}
		mod := m.mods[release.Name]
		if mod == nil {
			mod = &Mod{
				Name:     release.Name,
				releases: []*Release{},
			}
			m.mods[release.Name] = mod
		}
		mod.releases = append(mod.releases, release)
	}

	for _, mod := range m.mods {
		sort.Sort(mod.releases)
	}

	return nil
}

func entryExists(pathParts ...string) bool {
	_, err := os.Stat(path.Join(pathParts...))
	return err == nil
}

func isFactorioDir(dir string) bool {
	if !entryExists(dir, "data", "changelog.txt") {
		return false
	}
	if !entryExists(dir, "data", "base", "info.json") {
		return false
	}
	return entryExists(dir, "config-path.ini") || entryExists(dir, "config", "config.ini")
}