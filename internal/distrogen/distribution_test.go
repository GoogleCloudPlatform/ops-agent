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
	testdataSubpath              = filepath.Join("generator", "distribution")
	testdataFullDistributionPath = filepath.Join("testdata", "generator", "distribution")
)

func TestDistributionTemplateGeneration(t *testing.T) {
	registry, err := LoadEmbeddedRegistry()
	assert.NilError(t, err)

	testDirs, err := os.ReadDir(testdataFullDistributionPath)
	assert.NilError(t, err)
	for _, d := range testDirs {
		if !d.IsDir() {
			continue
		}

		name := d.Name()
		t.Run(name, func(t *testing.T) {
			testGeneratorCase(t, registry, name)
		})
	}
}

func testGeneratorCase(t *testing.T, registry *Registry, testFolder string) {
	specPath := filepath.Join(testdataFullDistributionPath, testFolder, "spec.yaml")

	d, err := NewDistributionSpec(specPath)
	assert.NilError(t, err)

	g, err := NewDistributionGenerator(d, registry, true)
	assert.NilError(t, err)

	// If custom templates exist for the test case, use them.
	customTemplates := filepath.Join(testdataFullDistributionPath, testFolder, "templates")
	if _, err := os.Stat(customTemplates); err == nil {
		g.CustomTemplatesDir = os.DirFS(customTemplates)
	}

	err = g.Generate()
	assert.NilError(t, err)
	t.Cleanup(func() {
		g.Clean()
	})

	goldenPath := filepath.Join(testdataFullDistributionPath, testFolder, "golden")
	goldenSubPath := filepath.Join(testdataSubpath, testFolder, "golden")
	assertGoldenFiles(t, g.GeneratePath, goldenPath, goldenSubPath)
}

func TestSpecValidationError(t *testing.T) {
	testCases := []struct {
		name        string
		expectedErr error
	}{
		{
			name:        "boringcrypto_alpine_build_container",
			expectedErr: ErrSpecValidationBoringCryptoWithoutDebian,
		},
		{
			name:        "boringcrypto_cgo_off",
			expectedErr: ErrSpecValidationBoringCryptoWithoutCGO,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testConfigPath := filepath.Join("testdata", "invalid", tc.name+".yaml")
			_, err := NewDistributionSpec(testConfigPath)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestSpecQuery(t *testing.T) {
	otelVer := "v0.124.0"
	spec := &DistributionSpec{
		OpenTelemetryVersion: otelVer,
	}
	val, err := spec.Query("opentelemetry_version")
	assert.NilError(t, err)
	assert.Equal(t, val, otelVer)
}

func TestSpecQueryNotFound(t *testing.T) {
	spec := &DistributionSpec{}
	_, err := spec.Query("random_field_name")
	assert.ErrorIs(t, err, ErrQueryValueNotFound)
}
