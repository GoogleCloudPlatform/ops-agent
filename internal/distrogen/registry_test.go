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
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"
)

func registryTestData(filename string) string { return filepath.Join("testdata", "registry", filename) }

func TestLoadEmbeddedRegistry(t *testing.T) {
	registry, err := LoadEmbeddedRegistry()
	assert.NilError(t, err)
	assert.Assert(t, registry != nil)
}

func TestLoadRegistry(t *testing.T) {
	registry, err := LoadRegistry(registryTestData("basic_registry.yaml"))
	assert.NilError(t, err)
	assert.Assert(t, registry != nil)
	assert.Assert(t, registry.Receivers["testreceiver"] != nil, "Expected 'testreceiver' to be loaded")
	assert.Equal(t, registry.Receivers["testreceiver"].GoMod.URL, "github.com/test/receiver")
	assert.Assert(t, registry.Processors["testprocessor"] != nil, "Expected 'testprocessor' to be loaded")
	assert.Equal(t, registry.Processors["testprocessor"].GoMod.URL, "github.com/test/processor")
	assert.Assert(t, registry.Exporters["testexporter"] != nil, "Expected 'testexporter' to be loaded")
	assert.Equal(t, registry.Exporters["testexporter"].GoMod.URL, "github.com/test/exporter")
	assert.Assert(t, registry.Connectors["testconnector"] != nil, "Expected 'testconnector' to be loaded")
	assert.Equal(t, registry.Connectors["testconnector"].GoMod.URL, "github.com/test/connector")
	assert.Assert(t, registry.Extensions["testextension"] != nil, "Expected 'testextension' to be loaded")
	assert.Equal(t, registry.Extensions["testextension"].GoMod.URL, "github.com/test/extension")
}

func TestLoadRegistry_Error(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		errCheck func(err error) bool
	}{
		{
			name: "Nonexistent File",
			path: "nonexistent_file.yaml",
			errCheck: func(err error) bool {
				return os.IsNotExist(err)
			},
		},
		{
			name: "Invalid YAML",
			path: "invalid_yaml.yaml",
			errCheck: func(err error) bool {
				// The yaml package doesn't return a typed error when the
				// parsing fails so we have to use an ugly string compare.
				// This yaml package is abandoned so it is unlikely to change
				// any time soon.
				return strings.Contains(err.Error(), "yaml: line")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadRegistry(registryTestData(tc.path))
			assert.Assert(t, tc.errCheck(err), "error check failed for %v", err)
		})
	}
}

func TestRegistry_Merge(t *testing.T) {
	testCases := []struct {
		name               string
		r1                 func() *Registry
		r2                 func() *Registry
		expectedReceivers  int
		expectedProcessors int
	}{
		{
			name: "Merge Different Components",
			r1: func() *Registry {
				r := NewRegistry()
				r.Receivers = RegistryComponents{
					"receiver1": {GoMod: &GoModuleID{URL: "github.com/r1"}},
				}
				return r
			},
			r2: func() *Registry {
				r := NewRegistry()
				r.Receivers = RegistryComponents{
					"receiver2": {GoMod: &GoModuleID{URL: "github.com/r2"}},
				}
				r.Processors = RegistryComponents{
					"processor1": {GoMod: &GoModuleID{URL: "github.com/p1"}},
				}
				return r
			},
			expectedReceivers:  2,
			expectedProcessors: 1,
		},
		{
			name: "Override Existing Component",
			r1: func() *Registry {
				r := NewRegistry()
				r.Receivers = RegistryComponents{
					"receiver1": {GoMod: &GoModuleID{URL: "github.com/r1"}},
				}
				return r
			},
			r2: func() *Registry {
				r := NewRegistry()
				r.Receivers = RegistryComponents{
					"receiver1": {GoMod: &GoModuleID{URL: "github.com/r2"}},
				}
				return r
			},
			expectedReceivers:  1,
			expectedProcessors: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r1 := tc.r1()
			r2 := tc.r2()
			r1.Merge(r2)
			assert.Equal(t, len(r1.Receivers), tc.expectedReceivers)
			assert.Equal(t, len(r1.Processors), tc.expectedProcessors)
		})
	}
}

func TestGoModuleID_String(t *testing.T) {
	testCases := []struct {
		name     string
		gm       *GoModuleID
		expected string
	}{
		{
			name:     "With Tag",
			gm:       &GoModuleID{URL: "github.com/test/module", Tag: "v1.2.3"},
			expected: "github.com/test/module v1.2.3",
		},
		{
			name:     "Without Tag",
			gm:       &GoModuleID{URL: "github.com/test/module"},
			expected: "github.com/test/module v0.0.0",
		},
		{
			name:     "Without Tag Allow Blank Tag",
			gm:       &GoModuleID{URL: "github.com/test/module", AllowBlankTag: true},
			expected: "github.com/test/module",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.gm.String(), tc.expected)
		})
	}
}

func TestGoModuleID_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expectedURL   string
		expectedTag   string
		expectedError bool
	}{
		{
			name:        "Valid Module ID with Tag",
			input:       "github.com/test/module v1.2.3",
			expectedURL: "github.com/test/module",
			expectedTag: "v1.2.3",
		},
		{
			name:        "Valid Module ID without Tag",
			input:       "github.com/test/module",
			expectedURL: "github.com/test/module",
			expectedTag: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gm GoModuleID
			node := yaml.Node{Value: tc.input}
			err := gm.UnmarshalYAML(&node)

			if tc.expectedError {
				assert.Error(t, err, "")
			} else {
				assert.NilError(t, err)
				assert.Equal(t, gm.URL, tc.expectedURL)
				assert.Equal(t, gm.Tag, tc.expectedTag)
			}
		})
	}
}

func TestGoModuleID_MarshalYAML(t *testing.T) {
	gm := &GoModuleID{URL: "github.com/test/module", Tag: "v1.2.3"}
	result, err := gm.MarshalYAML()
	assert.NilError(t, err)
	assert.Equal(t, result.(string), "github.com/test/module v1.2.3")
}

func TestRegistryComponent_RenderDocsURL(t *testing.T) {
	testCases := []struct {
		name     string
		docsURL  string
		expected string
	}{
		{
			name:     "With Docs URL",
			docsURL:  "https://example.com/docs",
			expected: "https://example.com/docs",
		},
		{
			name:     "Without Docs URL",
			docsURL:  "",
			expected: "No docs linked for component",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &RegistryComponent{DocsURL: tc.docsURL}
			assert.Equal(t, c.RenderDocsURL(), tc.expected)
		})
	}
}

func TestRegistryComponent_IsContrib(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "Is Contrib",
			url:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/test",
			expected: true,
		},
		{
			name:     "Is Not Contrib",
			url:      "github.com/test/module",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &RegistryComponent{GoMod: &GoModuleID{URL: tc.url}}
			assert.Equal(t, c.IsContrib(), tc.expected)
		})
	}
}

func TestRegistryComponent_ApplyOTelVersion(t *testing.T) {
	otelVersion := otelComponentVersion{
		core:       "1.2.3",
		coreStable: "1.0.0",
		contrib:    "0.5.0",
	}

	testCases := []struct {
		name     string
		url      string
		stable   bool
		expected string
	}{
		{
			name:     "Core Component",
			url:      "github.com/test/module",
			expected: "v1.2.3",
		},
		{
			name:     "Stable Core Component",
			url:      "github.com/test/module",
			stable:   true,
			expected: "v1.0.0",
		},
		{
			name:     "Contrib Component",
			url:      "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/test",
			expected: "v0.5.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &RegistryComponent{GoMod: &GoModuleID{URL: tc.url}, Stable: tc.stable}
			c.ApplyOTelVersion(otelVersion)
			assert.Equal(t, c.GoMod.Tag, tc.expected)
		})
	}
}

func TestRegistryComponent_GetOCBComponent(t *testing.T) {
	gm := &GoModuleID{URL: "github.com/test/module", Tag: "v1.2.3"}
	c := &RegistryComponent{GoMod: gm, Import: "github.com/test/module/test", Name: "test", Path: "test/path"}
	ocb := c.GetOCBComponent()
	assert.DeepEqual(t, ocb.GoMod, gm)
	assert.Equal(t, ocb.Import, c.Import)
	assert.Equal(t, ocb.Name, c.Name)
	assert.Equal(t, ocb.Path, c.Path)
}

func TestRegistryComponents_LoadAllComponents(t *testing.T) {
	otelVersion := otelComponentVersion{core: "1.2.3", coreStable: "1.0.0", contrib: "0.5.0"}
	rl := RegistryComponents{
		"component1": {GoMod: &GoModuleID{URL: "github.com/c1"}},
		"component2": {GoMod: &GoModuleID{URL: "github.com/c2"}},
	}
	names := []string{"component1", "component2", "component3"}
	components, errs := rl.LoadAllComponents(names, otelVersion)
	assert.Equal(t, len(components), 2)
	assert.Equal(t, len(errs), 1)
	assert.ErrorContains(t, errs["component3"], "component not found")
	assert.Equal(t, components["component1"].GoMod.Tag, "v1.2.3")
}

func TestRegistryComponents_LoadComponent(t *testing.T) {
	otelVersion := otelComponentVersion{core: "1.2.3", coreStable: "1.0.0", contrib: "0.5.0"}
	rl := RegistryComponents{
		"component1": {GoMod: &GoModuleID{URL: "github.com/c1"}},
	}
	component, err := rl.LoadComponent("component1", otelVersion)
	assert.NilError(t, err)
	assert.Equal(t, component.GoMod.URL, "github.com/c1")
	assert.Equal(t, component.GoMod.Tag, "v1.2.3")

	_, err = rl.LoadComponent("nonexistent", otelVersion)
	assert.ErrorIs(t, err, ErrComponentNotFound)
}

func TestRegistryComponents_Validate(t *testing.T) {
	rl := RegistryComponents{
		"component1": {GoMod: &GoModuleID{URL: "github.com/c1"}},
	}
	err := rl.Validate()
	assert.NilError(t, err)
}

func TestRegistryComponents_RenderOCBComponents(t *testing.T) {
	testCases := []struct {
		name     string
		rl       RegistryComponents
		expected string
	}{
		{
			name: "Multiple Components",
			rl: RegistryComponents{
				"component1": {GoMod: &GoModuleID{URL: "github.com/c1", Tag: "v1.0.0"}, Import: "github.com/c1/import", Name: "c1", Path: "c1/path"},
				"component2": {GoMod: &GoModuleID{URL: "github.com/a2", Tag: "v1.0.0"}, Import: "github.com/a2/import", Name: "a2", Path: "a2/path"},
			},
			expected: "github.com/a2",
		},
		{
			name:     "Empty Components",
			rl:       RegistryComponents{},
			expected: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.rl.RenderOCBComponents()
			if tc.expected == "" {
				assert.Equal(t, result, "")
			} else {
				assert.Assert(t, strings.Contains(result, tc.expected))
			}
		})
	}
}
