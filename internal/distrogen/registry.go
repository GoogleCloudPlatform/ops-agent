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
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var registryContent []byte

var ErrComponentNotFound = errors.New("component not found")

type ComponentType string

const (
	Receiver  ComponentType = "receiver"
	Processor ComponentType = "processor"
	Exporter  ComponentType = "exporter"
	Connector ComponentType = "connector"
	Extension ComponentType = "extension"
	Provider  ComponentType = "provider"
)

// Registry is a collection of components that can be used in
// a collector distribution.
type Registry struct {
	Receivers  RegistryComponents `yaml:"receivers"`
	Processors RegistryComponents `yaml:"processors"`
	Exporters  RegistryComponents `yaml:"exporters"`
	Connectors RegistryComponents `yaml:"connectors"`
	Extensions RegistryComponents `yaml:"extensions"`
	Providers  RegistryComponents `yaml:"providers"`
	Path       string             `yaml:"-"`
}

// NewRegistry will create an empty registry object with the
// component lists preallocated.
func NewRegistry() *Registry {
	return &Registry{
		Receivers:  RegistryComponents{},
		Processors: RegistryComponents{},
		Exporters:  RegistryComponents{},
		Connectors: RegistryComponents{},
		Extensions: RegistryComponents{},
		Providers:  RegistryComponents{},
	}
}

// LoadEmbeddedRegistry will load the registry embedded in the
// distrogen binary.
func LoadEmbeddedRegistry() (*Registry, error) {
	var r Registry
	if err := yaml.Unmarshal(registryContent, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// LoadRegistry will load a registry from a yaml file.
func LoadRegistry(path string) (*Registry, error) {
	r := NewRegistry()
	err := yamlUnmarshalFromFileInto(path, r)
	r.Path = path
	return r, err
}

// Merge will merge another registry into this one. If the provided
// registry contains any of the same entry keys as the current
// registry, it will be overridden.
func (r *Registry) Merge(r2 *Registry) {
	mapMerge(r.Receivers, r2.Receivers)
	mapMerge(r.Processors, r2.Processors)
	mapMerge(r.Exporters, r2.Exporters)
	mapMerge(r.Connectors, r2.Connectors)
	mapMerge(r.Extensions, r2.Extensions)
	mapMerge(r.Providers, r2.Providers)
}

func (r *Registry) Add(componentType ComponentType, component *RegistryComponent) {
	switch componentType {
	case Receiver:
		r.Receivers[component.Name] = component
	case Processor:
		r.Processors[component.Name] = component
	case Exporter:
		r.Exporters[component.Name] = component
	case Connector:
		r.Connectors[component.Name] = component
	case Extension:
		r.Extensions[component.Name] = component
	case Provider:
		r.Providers[component.Name] = component
	}
}

func (r *Registry) Save() error {
	if r.Path == "" {
		return errors.New("cannot save registry: no path set")
	}

	if err := yamlMarshalToFile(r, r.Path, DefaultProjectFileMode); err != nil {
		return err
	}

	return nil
}

// GoModuleID is intended for stringifying/unmarshalling to
// a Go module ID, i.e. github.com/package/name v0.0.0 format.
type GoModuleID struct {
	URL           string
	Tag           string
	AllowBlankTag bool
}

// String outputs the GoModuleID details in proper format.
func (gm *GoModuleID) String() string {
	tag := gm.Tag
	if tag == "" {
		// There are certain cases (like local paths) where
		// the tag for a Go Module ID is allowed to be blank.
		if gm.AllowBlankTag {
			return gm.URL
		}

		// Otherwise if there is no tag specified, then it is assumed that this module
		// will be replaced. Use the tag v0.0.0 since it will be ignored
		// in the replace anyway.
		logger.Debug("no tag detected for module, using v0.0.0", slog.String("module", gm.URL))
		tag = "v0.0.0"
	}
	return fmt.Sprintf("%s %s", gm.URL, tag)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
// It takes a properly formed Go Module ID string and unpacks
// it into the struct.
func (gm *GoModuleID) UnmarshalYAML(value *yaml.Node) error {
	// The module ID may have a version.
	moduleStr := value.Value
	moduleComponents := strings.Split(moduleStr, " ")
	gm.URL = moduleComponents[0]
	if len(moduleComponents) > 1 {
		gm.Tag = moduleComponents[1]
	}
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface. It leverages
// the String method to allow outputting the value into a YAML document
// in the module ID string form.
func (gm *GoModuleID) MarshalYAML() (interface{}, error) {
	return gm.String(), nil
}

type otelComponentVersion struct {
	core       string
	coreStable string
	contrib    string
}

// RegistryComponent is the type used as a basis for Registry.
// It contains all the information needed to output a
type RegistryComponent struct {
	Name string `yaml:"-"`

	GoMod         *GoModuleID `yaml:"gomod"`
	Import        string      `yaml:"import,omitempty"`
	Path          string      `yaml:"path,omitempty"`
	Stable        bool        `yaml:"stable,omitempty"`
	StartRevision string      `yaml:"start_revision,omitempty"`
	DocsURL       string      `yaml:"docs_url,omitempty"`
}

// RenderDocsURL renders the docs URL into a template.
func (c *RegistryComponent) RenderDocsURL() string {
	if c.DocsURL == "" {
		return "No docs linked for component"
	}
	return c.DocsURL
}

// IsContrib determines whether the module comes from the opentelemetry-collector-contrib repo.
func (c *RegistryComponent) IsContrib() bool {
	return strings.Contains(c.GoMod.URL, "github.com/open-telemetry/opentelemetry-collector-contrib")
}

func (c *RegistryComponent) ApplyOTelVersion(otelVersion otelComponentVersion) {
	c.GoMod.Tag = "v" + otelVersion.core
	if c.Stable {
		c.GoMod.Tag = "v" + otelVersion.coreStable
	} else if c.IsContrib() {
		c.GoMod.Tag = "v" + otelVersion.contrib
	}
}

// OCBManifestComponent is a reflection of the fields for an
// entry in an OCB manifest yaml.
type OCBManifestComponent struct {
	GoMod  *GoModuleID `yaml:"gomod"`
	Import string      `yaml:"import,omitempty"`
	Name   string      `yaml:"string,omitempty"`
	Path   string      `yaml:"path,omitempty"`
}

// GetOCBComponent will return an OCBManifestComponent using
// the details from this RegistryComponent.
func (c *RegistryComponent) GetOCBComponent() OCBManifestComponent {
	return OCBManifestComponent{
		GoMod:  c.GoMod,
		Import: c.Import,
		Name:   c.Name,
		Path:   c.Path,
	}
}

// RegistryComponentRelease is a particular tag of a component that declares
// the Collector library version it supports.
type RegistryComponentRelease struct {
	Tag                         string `yaml:"version"`
	OpenTelemetryVersion        string `yaml:"opentelemetry_version"`
	OpenTelemetryContribVersion string `yaml:"opentelemetry_contrib_version,omitempty"`
}

// RegistryComponents is a map of registry component names to component
// details.
type RegistryComponents map[string]*RegistryComponent

// LoadAllComponents will take a list of component names and load them
// from the registry, attaching the appropriate version tag.
func (rl RegistryComponents) LoadAllComponents(names []string, otelVersion otelComponentVersion) (RegistryComponents, CollectionError) {
	components := RegistryComponents{}
	errs := make(CollectionError)

	for _, name := range names {
		entry, err := rl.LoadComponent(name, otelVersion)
		if err != nil {
			errs[name] = ErrComponentNotFound
			continue
		}
		components[name] = entry
	}

	return components, errs
}

func (rl RegistryComponents) LoadComponent(name string, otelVersion otelComponentVersion) (*RegistryComponent, error) {
	entry, ok := rl[name]
	if !ok {
		return nil, ErrComponentNotFound
	}
	entry.ApplyOTelVersion(otelVersion)
	return entry, nil
}

// Validate is intended to be called before template rendering.
// This way, calling the Render method from the template can assume
// no error.
func (cs RegistryComponents) Validate() error {
	_, err := yaml.Marshal(cs)
	return err
}

// RenderOCBComponents will render the registry entries as
func (cs RegistryComponents) RenderOCBComponents() string {
	if len(cs) == 0 {
		return ""
	}

	renderComponents := []OCBManifestComponent{}
	for _, c := range cs {
		renderComponents = append(renderComponents, c.GetOCBComponent())
	}

	// The component list is sorted here to ensure that re-generating will always
	// have a consistent order.
	slices.SortFunc(renderComponents, func(a OCBManifestComponent, b OCBManifestComponent) int {
		return strings.Compare(a.GoMod.URL, b.GoMod.URL)
	})

	return renderYaml(renderComponents)
}
