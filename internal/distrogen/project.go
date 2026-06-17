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

var (
	DefaultProjectFileMode = fs.ModePerm
)

type ProjectGenerator struct {
	Spec       *DistributionSpec
	FileMode   fs.FileMode
	CustomPath string
}

func NewProjectGenerator(spec *DistributionSpec) (*ProjectGenerator, error) {
	return &ProjectGenerator{
		Spec:     spec,
		FileMode: DefaultProjectFileMode,
	}, nil
}

func (pg *ProjectGenerator) Generate() error {
	makeTemplates, err := GetMakeTemplateSet(pg, pg.FileMode)
	if err != nil {
		logger.Debug("failed to get make templates", "err", err)
		return err
	}
	projectTemplates, err := GetProjectTemplateSet(pg, pg.FileMode)
	if err != nil {
		logger.Debug("failed to get project templates", "err", err)
		return err
	}
	distrogenTemplateSet, err := GetDistrogenTemplateSet(pg, pg.FileMode)
	if err != nil {
		logger.Debug("failed to get component templates", "err", err)
		return err
	}
	scriptTemplateSet, err := GetScriptTemplateSet(pg, pg.FileMode)
	if err != nil {
		logger.Debug("failed to get script templates", "err", err)
		return err
	}

	crg := NewComponentsRegistryGenerator()

	generatePath := "."
	if pg.CustomPath != "" {
		generatePath = pg.CustomPath
		crg.Path = generatePath
	}

	if err := crg.Generate(); err != nil {
		return err
	}

	generateMakePath := filepath.Join(generatePath, "make")
	generateScriptsPath := filepath.Join(generatePath, "scripts")

	var dirErrors []error
	if err := os.MkdirAll(generateMakePath, pg.FileMode); err != nil {
		dirErrors = append(dirErrors, err)
	}
	if err := os.MkdirAll(filepath.Join(generatePath, "templates"), pg.FileMode); err != nil {
		dirErrors = append(dirErrors, err)
	}
	if err := os.MkdirAll(generateScriptsPath, pg.FileMode); err != nil {
		dirErrors = append(dirErrors, err)
	}
	if _, err := os.Create(filepath.Join(generatePath, "templates", EMPTY_FILE_NAME)); err != nil {
		dirErrors = append(dirErrors, err)
	}

	if len(dirErrors) > 0 {
		return errors.Join(dirErrors...)
	}

	if err := GenerateTemplateSet(generateMakePath, makeTemplates); err != nil {
		return err
	}
	if err := GenerateTemplateSet(generateScriptsPath, scriptTemplateSet); err != nil {
		return err
	}
	if err := GenerateTemplateSet(filepath.Join(generatePath, "."), projectTemplates); err != nil {
		return err
	}
	if err := GenerateTemplateSet(filepath.Join(generatePath, ".distrogen"), distrogenTemplateSet); err != nil {
		return err
	}

	return nil
}
