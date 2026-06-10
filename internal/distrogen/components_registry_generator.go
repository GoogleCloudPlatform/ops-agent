// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

var componentRegistryPath = filepath.Join("components", "registry.yaml")

type ComponentsRegistryGenerator struct {
	FileMode fs.FileMode

	Path string
}

func NewComponentsRegistryGenerator() *ComponentsRegistryGenerator {
	g := &ComponentsRegistryGenerator{
		FileMode: DefaultProjectFileMode,
		Path:     ".",
	}
	return g
}

// The component registry is for custom components built within this repository.
// Upstream components are managed in the internal registry at cmd/distrogen/registry.yaml.
func (g *ComponentsRegistryGenerator) Generate() error {
	registry := NewRegistry()

	generateComponentsPath := filepath.Join(g.Path, "components")
	registry.Path = filepath.Join(generateComponentsPath, "registry.yaml")

	var dirErrors []error
	if err := os.MkdirAll(generateComponentsPath, g.FileMode); err != nil {
		dirErrors = append(dirErrors, err)
	}

	if len(dirErrors) > 0 {
		return errors.Join(dirErrors...)
	}

	if err := registry.Save(); err != nil {
		return err
	}

	templates, err := GetComponentsTemplateSet(g, g.FileMode)
	if err != nil {
		return err
	}

	if err := GenerateTemplateSet(generateComponentsPath, templates); err != nil {
		logger.Debug("failed to get component templates", "err", err)
		return err
	}
	return nil
}
