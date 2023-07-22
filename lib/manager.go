package fmm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

var internalMods = map[string]bool{
	"base": true,
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

// Creates a new Manager for the given game directory. A game directory is
// considered valid if it has either a config-path.cfg file or a
// config/config.ini file. The player's username and token will
// be automatically retrieved from `player-data.json` if it exists.
func NewManager(gamePath string) (*Manager, error) {
	if !isFactorioDir(gamePath) {
		return nil, errors.New("invalid Factorio data directory")
	}

	m := Manager{
		DoSave:          true,
		gamePath:        gamePath,
		modListJsonPath: filepath.Join(gamePath, "mods", "mod-list.json"),
		mods:            mods{},
		modsPath:        filepath.Join(gamePath, "mods"),
	}

	if err := m.readPlayerData(); err != nil {
		return nil, errors.Join(errors.New("unable to get player data"), err)
	}

	modListJsonPath := filepath.Join(m.modsPath, "mod-list.json")
	if !entryExists(m.modsPath) {
		if err := os.Mkdir("mods", 0755); err != nil {
			return nil, errors.Join(errors.New("failed to create mods directory"), err)
		}
		modListJson := modListJson{
			Mods: []modListJsonMod{
				{Name: "base", Enabled: true},
			},
		}
		data, _ := json.Marshal(modListJson)
		if err := os.WriteFile(modListJsonPath, data, fs.ModeExclusive); err != nil {
			return nil, errors.Join(errors.New("failed to create mod-list.json"), err)
		}
	}

	if err := m.parseMods(); err != nil {
		return nil, errors.Join(errors.New("error parsing mods"), err)
	}

	modListJsonData, err := os.ReadFile(modListJsonPath)
	if err != nil {
		return nil, errors.Join(errors.New("error reading mod-list.json"), err)
	}
	var modListJson modListJson
	if err = json.Unmarshal(modListJsonData, &modListJson); err != nil {
		return nil, errors.Join(errors.New("error parsing mod-list.json"), err)
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

	return &m, nil
}

// Requests the mod to be disabled.
func (m *Manager) Disable(modName string) error {
	mod, err := m.GetMod(modName)
	if err != nil {
		return err
	}
	if mod.Enabled == nil {
		return errors.New("mod is already disabled")
	}
	mod.Enabled = nil
	return nil
}

// Requests all non-internal mods to be disabled.
func (m *Manager) DisableAll() {
	for _, mod := range m.mods {
		if !internalMods[mod.Name] {
			mod.Enabled = nil
		}
	}
}

// Requests the mod to be enabled. If version is nil, it will default to the
// newest available release.
func (m *Manager) Enable(name string, version *Version) error {
	mod, err := m.GetMod(name)
	if err != nil {
		return err
	}
	release := mod.GetRelease(version)
	if release == nil {
		return errors.New("unable to find a matching release")
	}
	enabled := release.Version
	mod.Enabled = &enabled
	return nil
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

// Retrives the current API key, used for uploading mods.
func (m *Manager) GetApiKey() string {
	return m.apiKey
}

// Sets the API key, used for uploading mods.
func (m *Manager) SetApiKey(key string) {
	m.apiKey = key
}

// Returns true if the Manager has the player's username and token data.
func (m *Manager) HasPlayerData() bool {
	return m.downloadUsername != "" && m.downloadToken != ""
}

// Sets the player's username and token data.
func (m *Manager) SetPlayerData(username string, token string) {
	m.downloadUsername = username
	m.downloadToken = token
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
		return errors.Join(errors.New("could not read mods directory"), err)
	}

	for _, entry := range entries {
		filename := entry.Name()
		if filename == "mod-list.json" || filename == "mod-settings.dat" {
			continue
		}
		release, err := releaseFromFile(filepath.Join(m.modsPath, filename))
		if err != nil {
			return errors.Join(errors.New(fmt.Sprint("unable to parse ", filename)), err)
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
	_, err := os.Stat(filepath.Join(pathParts...))
	return err == nil
}

func isFactorioDir(dir string) bool {
	return entryExists(dir, "config-path.cfg") || entryExists(dir, "config", "config.ini")
}
