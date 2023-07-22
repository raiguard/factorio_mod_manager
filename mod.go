package main

type Mod struct {
	Enabled  *Version
	Name     string
	releases Releases
}

type Releases []*Release

func (m *Mod) getLatestRelease() *Release {
	return m.releases[len(m.releases)-1]
}

func (m *Mod) getRelease(version *Version) *Release {
	if version == nil {
		return m.getLatestRelease()
	}
	return m.getMatchingRelease(&Dependency{
		m.Name,
		version,
		DependencyRequired,
		VersionEq,
	})
}

func (m *Mod) getMatchingRelease(dep *Dependency) *Release {
	// Iterate in reverse to get the newest version first
	for i := len(m.releases) - 1; i >= 0; i-- {
		release := m.releases[i]
		if dep.Test(&release.Version) {
			return release
		}
	}
	return nil
}

// Implementations for sorting interface
// TODO: Use Go 1.21 `slices` module once it is released
func (r Releases) Len() int {
	return len(r)
}
func (r Releases) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
func (r Releases) Less(i, j int) bool {
	releaseI, releaseJ := r[i], r[j]
	return releaseI.Version.cmp(&releaseJ.Version) == VersionLt
}
