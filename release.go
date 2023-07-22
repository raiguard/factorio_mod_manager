package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Release struct {
	Name         string
	Dependencies []*Dependency
	Path         string
	Version      Version
}

func releaseFromFile(path string) (*Release, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.Join(errors.New("Unable to get file info"), err)
	}
	filename := filepath.Base(path)
	var infoJson infoJson
	if info.Mode().IsRegular() {
		infoJson, err = readZipInfoJson(path)
	} else if info.IsDir() || isSymlink(info) {
		file, err := os.Open(filepath.Join(path, "info.json"))
		if err == nil {
			infoJson, err = readInfoJson(file)
		}
	}

	if err != nil {
		return nil, errors.Join(errors.New("Error when parsing info.json"), err)
	}

	expectedFilename := fmt.Sprintf("%s_%s.zip", infoJson.Name, infoJson.Version.toString(false))
	if filename != expectedFilename && (info.Mode().IsRegular() || filename != infoJson.Name) {
		return nil, errors.New(fmt.Sprint("Release filename does not match the expected filename", expectedFilename))
	}

	return &Release{
		infoJson.Name,
		infoJson.Dependencies,
		filename,
		infoJson.Version,
	}, nil
}

type infoJson struct {
	Dependencies []*Dependency `json:"dependencies"`
	Name         string        `json:"name"`
	Version      Version       `json:"version"`
}

func isSymlink(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink > 0
}

func readInfoJson(rc io.ReadCloser) (infoJson, error) {
	var infoJson infoJson

	content, err := io.ReadAll(rc)
	if err != nil {
		return infoJson, err
	}

	err = json.Unmarshal(content, &infoJson)
	return infoJson, err
}

func readZipInfoJson(path string) (infoJson, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return infoJson{}, err
	}

	var file *zip.File

	for _, existing := range r.File {
		parts := strings.Split(existing.Name, "/")
		if len(parts) == 2 && parts[1] == "info.json" {
			file = existing
			break
		}
	}

	if file == nil {
		return infoJson{}, errors.New("Could not locate info.json file")
	}

	rc, err := file.Open()
	if err != nil {
		return infoJson{}, err
	}
	defer rc.Close()

	return readInfoJson(rc)
}