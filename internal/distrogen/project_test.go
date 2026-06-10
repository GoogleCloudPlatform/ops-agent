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
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

var (
	testdataFullProjectPath = filepath.Join("testdata", "generator", "project")
)

func TestProjectTemplateGeneration(t *testing.T) {
	registry, err := LoadEmbeddedRegistry()
	assert.NilError(t, err)

	t.Run("project", func(t *testing.T) {
		testProjectGeneratorCase(t, registry, "project")
	})
}

func testProjectGeneratorCase(t *testing.T, registry *Registry, testFolder string) {
	specPath := filepath.Join(testdataFullProjectPath, "spec.yaml")

	// Create a temporary directory to generate files in, to avoid polluting testdata.
	tempDir, err := os.MkdirTemp("", "project-generator-test")
	assert.NilError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	// The generator expects the spec file to be in the project path, so copy it there.
	specData, err := os.ReadFile(specPath)
	assert.NilError(t, err)
	tempSpecPath := filepath.Join(tempDir, "spec.yaml")
	err = os.WriteFile(tempSpecPath, specData, 0644)
	assert.NilError(t, err)

	d, err := NewDistributionSpec(tempSpecPath)
	assert.NilError(t, err)

	p, err := NewProjectGenerator(d)
	assert.NilError(t, err)
	p.CustomPath = tempDir

	err = p.Generate()
	assert.NilError(t, err)

	goldenPath := filepath.Join(testdataFullProjectPath, "golden")
	goldenSubPath := filepath.Join("generator", "project", "golden")
	assertGoldenFiles(t, p.CustomPath, goldenPath, goldenSubPath)
}
