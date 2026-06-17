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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var errInvalidComponentType = errors.New("invalid component type")

type ComponentGenerator struct {
	Spec     *DistributionSpec
	FileMode fs.FileMode

	Type      ComponentType
	Name      string
	Path      string
	ModuleURL string
}

func NewComponentGenerator(spec *DistributionSpec, componentType ComponentType, componentName string) (*ComponentGenerator, error) {
	if spec.ComponentModuleBase == "" {
		return nil, errors.New("must supply a component_module_base in spec")
	}

	g := &ComponentGenerator{
		Spec:     spec,
		FileMode: DefaultProjectFileMode,
		Type:     componentType,
		Name:     componentName,
	}

	switch componentType {
	case Receiver:
		fallthrough
	case Processor:
		fallthrough
	case Exporter:
		fallthrough
	case Connector:
		fallthrough
	case Extension:
		fallthrough
	case Provider:
		g.Path = filepath.Join(
			"components",
			string(componentType),
			fmt.Sprintf("%s%s", componentName, componentType),
		)

	default:
		return nil, fmt.Errorf("%w: %s", errInvalidComponentType, componentType)
	}

	g.ModuleURL = fmt.Sprintf("%s/%s", spec.Module, g.Path)

	return g, nil
}

func (g *ComponentGenerator) Generate() error {
	componentTemplates, err := GetIndividualComponentTemplateSet(g, g.FileMode)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(g.Path, g.FileMode); err != nil {
		return err
	}

	if err := GenerateTemplateSet(g.Path, componentTemplates); err != nil {
		return err
	}

	registryPath := componentRegistryPath
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		registry = NewRegistry()
		registry.Path = registryPath
	}

	registry.Add(g.Type, &RegistryComponent{
		GoMod: &GoModuleID{
			URL:           g.ModuleURL,
			AllowBlankTag: true,
		},
		Name: g.Name,
		Path: "../" + g.Path,
	})
	if err := registry.Save(); err != nil {
		return err
	}

	return nil
}
