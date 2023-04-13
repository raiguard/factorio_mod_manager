package main

import (
	"fmt"
	"path"
	"strings"
)

func parseMods(input []string, parseDependencies bool) []ModIdent {
	var output []ModIdent

	for _, input := range input {
		if strings.HasSuffix(input, ".zip") {
			// TODO: Read from save
		} else if strings.HasSuffix(input, ".log") {
			output = append(output, parseLogFile(input)...)
		} else if strings.HasSuffix(input, ".json") {
			// TODO: info.json
		} else if strings.HasPrefix(input, "!") {
			// TODO: Mod set
		} else {
			output = append(output, newModIdent(input))
		}
	}

	if parseDependencies {
		output = expandDependencies(output)
	}

	return output
}

func expandDependencies(mods []ModIdent) []ModIdent {
	visited := make(map[string]bool)

	dir, err := newDir(modsDir)
	if err != nil {
		abort(err)
	}

	for i := 0; i < len(mods); i += 1 {
		mod := mods[i]
		visited[mod.Name] = true
		file, _, err := dir.Find(Dependency{mod, DependencyRequired, VersionAny})
		var deps []Dependency
		if file != nil {
			realDeps, err := file.Dependencies()
			if err != nil {
				errorln(err)
				continue
			}
			deps = *realDeps
		} else {
			deps, err = portalGetDependencies(mod)
			if err != nil {
				errorln(err)
				continue
			}
		}
		for _, dep := range deps {
			// FIXME: Dependency kind or version might be different / incompatible
			if dep.Ident.Name == "base" || visited[dep.Ident.Name] {
				continue
			}
			if dep.Kind == DependencyRequired || dep.Kind == DependencyNoLoadOrder {
				visited[dep.Ident.Name] = true
				mods = append(mods, dep.Ident)
			}
		}
	}

	return mods
}

func disable(args []string) {
	if len(args) == 0 {
		disableAll()
		return
	}

	dir, err := newDir(modsDir)
	if err != nil {
		abort(err)
	}
	defer dir.save()

	var mods []Dependency
	for _, input := range args {
		mods = append(mods, Dependency{Ident: newModIdent(input), Req: VersionAny})
	}
	for _, mod := range mods {
		file, entry, err := dir.Find(mod)
		if err != nil {
			errorln(err)
			continue
		}
		if !entry.Enabled {
			continue
		}
		entry.Enabled = false
		fmt.Println("Disabled", file.Ident.Name)
	}
}

func disableAll() {
	list, err := newModList(path.Join(modsDir, "mod-list.json"))
	if err != nil {
		abort(err)
	}
	defer list.save()

	for i := range list.Mods {
		mod := &list.Mods[i]
		if mod.Name != "base" {
			mod.Enabled = false
		}
	}

	fmt.Println("Disabled all mods")
}

func enable(args []string) {
	if len(args) == 0 {
		abort("no mods were provided")
	}

	dir, err := newDir(modsDir)
	if err != nil {
		abort(err)
	}
	defer dir.save()

	var mods []Dependency
	for _, input := range args {
		mods = append(mods, Dependency{Ident: newModIdent(input), Req: VersionEq})
	}

	i := 0
	for {
		if i > len(mods)-1 {
			break
		}
		mod := mods[i]
		i++
		file, entry, err := dir.Find(mod)
		if err != nil {
			errorln(err)
			continue
		}
		if entry.Enabled {
			if mod.Ident.Version == nil || mod.Ident.Version.cmp(entry.Version) == VersionEq {
				continue
			}
		}
		entry.Enabled = true
		entry.Version = mod.Ident.Version
		fmt.Println("Enabled", file.Ident.toString())

		deps, err := file.Dependencies()
		if err != nil {
			errorln(err)
		}
		if deps == nil {
			continue
		}

		for _, dep := range *deps {
			if dep.Ident.Name != "base" && dep.Kind == DependencyRequired {
				mods = append(mods, dep)
			}
		}
	}
}

func install(args []string) {
	if len(args) == 0 {
		abort("no mods were provided")
	}

	if downloadUsername == "" {
		abort("Username not specified")
	}
	if downloadToken == "" {
		abort("Token not specified")
	}

	dir, err := newDir(modsDir)
	if err != nil {
		abort(err)
	}

	var mods []Dependency
	for _, input := range args {
		mods = append(mods, Dependency{Ident: newModIdent(input), Req: VersionEq})
	}

	for _, mod := range mods {
		// TODO: Do we want to do this?
		if file, _, _ := dir.Find(mod); file != nil {
			fmt.Println(file.Ident.toString(), "is already in the mods directory")
			continue
		}

		err := portalDownloadMod(mod, dir)
		if err != nil {
			errorln(err)
		}
	}
}

func sync(files []string) {
	for _, file := range files {
		if strings.HasSuffix(file, ".log") {
			if err := parseLogFile(file); err != nil {
				errorln(err)
			}
		}
	}
}

func upload(files []string) {
	if apiKey == "" {
		abort("API key not specified.")
	}
	if len(files) == 0 {
		abort("no files were provided")
	}
	for _, file := range files {
		if err := portalUploadMod(file); err != nil {
			abort("Upload failed:", err)
		}
	}
}
