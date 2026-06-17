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
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
)

var ErrNoDiff = errors.New("no differences found with previous generation")

// -rw-r--r--
var DefaultFileMode fs.FileMode = 0644

type BuildContainerOption string

const (
	Alpine BuildContainerOption = "alpine"
	Debian BuildContainerOption = "debian"
)

// DistributionSpec is the specification for a new OpenTelemetry Collector distribution.
// It contains all the information that will be formatted into the default set of
// templates/user provided templates.
type DistributionSpec struct {
	Path                        string                  `yaml:"-"`
	Name                        string                  `yaml:"name"`
	Module                      string                  `yaml:"module"`
	DisplayName                 string                  `yaml:"display_name"`
	Description                 string                  `yaml:"description"`
	Blurb                       string                  `yaml:"blurb"`
	BuildContainer              BuildContainerOption    `yaml:"build_container"`
	Version                     string                  `yaml:"version"`
	OpenTelemetryVersion        string                  `yaml:"opentelemetry_version"`
	OpenTelemetryContribVersion string                  `yaml:"opentelemetry_contrib_version"`
	OpenTelemetryStableVersion  string                  `yaml:"opentelemetry_stable_version"`
	GoVersion                   string                  `yaml:"go_version"`
	BinaryName                  string                  `yaml:"binary_name"`
	BuildTags                   string                  `yaml:"build_tags"`
	BoringCrypto                bool                    `yaml:"boringcrypto"`
	DockerRepo                  string                  `yaml:"docker_repo"`
	Components                  *DistributionComponents `yaml:"components"`
	Replaces                    ComponentReplaces       `yaml:"replaces,omitempty"`
	CustomValues                map[string]any          `yaml:"custom_values,omitempty"`
	FeatureGates                FeatureGates            `yaml:"feature_gates,omitempty"`
	GoProxy                     string                  `yaml:"go_proxy,omitempty"`

	// CollectorCGO determines whether the Collector will be built with CGO.
	CollectorCGO        bool   `yaml:"collector_cgo,omitempty"`
	ComponentModuleBase string `yaml:"component_module_base"`
	DistrogenVersion    string `yaml:"distrogen_version"`
}

// RenderGoMajorVersion will parse the GoVersion in the spec and return a version without a patch
func (s *DistributionSpec) RenderGoMajorVersion() string {
	split := strings.Split(s.GoVersion, ".")
	if len(split) < 2 {
		return s.GoVersion
	}
	return fmt.Sprintf("%s.%s", split[0], split[1])
}

// Diff will compare two different DistributionSpecs.
func (s *DistributionSpec) Diff(s2 *DistributionSpec) bool {
	diff := cmp.Diff(s, s2)
	return diff != ""
}

var ErrQueryValueNotFound = errors.New("not found in spec")
var ErrQueryValueInvalid = errors.New("found in spec but unsupported type")

// Query will get a field from a loaded spec based on the yaml
// field name.
func (s *DistributionSpec) Query(field string) (string, error) {
	v := reflect.ValueOf(s).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		structField := t.Field(i)
		yamlTag := structField.Tag.Get("yaml")

		// Handle tags like "replaces,omitempty"
		tagName := strings.Split(yamlTag, ",")[0]

		if tagName == field {
			fieldValue := v.Field(i)
			// Convert the field value to string.
			// This handles basic types like string, int, bool.
			if fieldValue.IsValid() && fieldValue.CanInterface() {
				return fmt.Sprintf("%v", fieldValue.Interface()), nil
			}
			return "", fmt.Errorf("field '%s': %w", field, ErrQueryValueInvalid)
		}
	}
	return "", fmt.Errorf("field '%s': %w", field, ErrQueryValueNotFound)
}

var (
	ErrSpecValidationBoringCryptoWithoutCGO    = errors.New("boringcrypto build is not possible with collector_cgo turned off")
	ErrSpecValidationBoringCryptoWithoutDebian = errors.New("boringcrypto is only possible with the debian build container")
)

// NewDistributionSpec loads the DistributionSpec from a yaml file.
func NewDistributionSpec(path string) (*DistributionSpec, error) {
	spec, err := yamlUnmarshalFromFile[DistributionSpec](path)
	if err != nil {
		return nil, err
	}

	if spec.BuildContainer == "" {
		spec.BuildContainer = Debian
	}

	// If BoringCrypto is set, CGO must be enabled and only the debian
	// build container can be used.
	if spec.BoringCrypto {
		// If CGO was manually set to false in the config, return a validation error.
		if !spec.CollectorCGO {
			return nil, ErrSpecValidationBoringCryptoWithoutCGO
		}
		// If build container is manually set to something other than debian, return a validation error.
		if spec.BuildContainer != Debian {
			return nil, fmt.Errorf("%w, build_container was set to %s", ErrSpecValidationBoringCryptoWithoutDebian, spec.BuildContainer)
		}
	}

	// The name of the spec.yaml file might be different from the binary name
	spec.Path = filepath.Base(path)

	// It is a rare case where the contrib version falls out of sync with
	// the canonical OpenTelemetry version, most of the time it is the same.
	if spec.OpenTelemetryContribVersion == "" {
		spec.OpenTelemetryContribVersion = spec.OpenTelemetryVersion
	}

	return spec, nil
}

// DistributionComponents is a set of components with RegistryComponent names
// that defines all the components included in this collector distribution.
type DistributionComponents struct {
	Receivers  []string `yaml:"receivers,omitempty"`
	Processors []string `yaml:"processors,omitempty"`
	Exporters  []string `yaml:"exporters,omitempty"`
	Connectors []string `yaml:"connectors,omitempty"`
	Extensions []string `yaml:"extensions,omitempty"`
	Providers  []string `yaml:"providers,omitempty"`
}

// DistributionGenerator contains all the facilities to generate a distribution
// from a DistributionSpec.
type DistributionGenerator struct {
	Spec               *DistributionSpec
	GenerateDirName    string
	GeneratePath       string
	Registry           *Registry
	CustomTemplatesDir fs.FS
	FileMode           fs.FileMode
}

// NewDistributionGenerator creates a DistributionGenerator.
func NewDistributionGenerator(spec *DistributionSpec, registry *Registry, forceGenerate bool) (*DistributionGenerator, error) {
	d := DistributionGenerator{
		Spec:     spec,
		Registry: registry,
		// -rw-r--r--
		FileMode: DefaultFileMode,
	}
	d.GenerateDirName = spec.Name

	if !forceGenerate {
		specCache, err := yamlUnmarshalFromFile[DistributionSpec](filepath.Join(d.GenerateDirName, "spec.yaml"))
		if err != nil {
			logger.Debug(fmt.Sprintf("generated spec could not be read: %v", err))
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			if !d.Spec.Diff(specCache) {
				return nil, ErrNoDiff
			}
		}
	}

	tmpDir, err := os.MkdirTemp(".", d.GenerateDirName)
	if err != nil {
		return nil, err
	}
	d.GeneratePath = tmpDir
	return &d, nil
}

// Generate will generate the distribution. It will generate the distribution
// in a temporary local directory, and upon there no errors in the generation
// will move it into the destination path.
func (d *DistributionGenerator) Generate() error {
	templateContext, err := NewTemplateContextFromSpec(d.Spec, d.Registry)
	if err != nil {
		return err
	}
	templates, err := GetDistributionTemplateSet(templateContext, d.FileMode)
	if err != nil {
		return err
	}

	if d.CustomTemplatesDir != nil {
		customTemplates, err := GetTemplateSetFromDir(d.CustomTemplatesDir, templateContext, d.FileMode)
		if err != nil {
			return err
		}

		// This merge means that any custom templates named the same as the embedded
		// defaults will overwrite the embedded version with the custom version.
		mapMerge(templates, customTemplates)
	}

	templates.RenameExceptionalTemplates(d.Spec)
	for _, tmpl := range templates {
		if err := tmpl.Render(d.GeneratePath); err != nil {
			return err
		}
	}
	if err := d.WriteSpec(); err != nil {
		return err
	}

	return nil
}

// WriteSpec renders the DistributionSpec in a yaml file that lives in the generated
// distribution. This is a human readable way to keep track of what spec was used for
// this existing generation, as well as a method of detecting whether a new generation
// needs to be done at all (if no spec changes no need to generate).
func (d *DistributionGenerator) WriteSpec() error {
	return yamlMarshalToFile(d.Spec, filepath.Join(d.GeneratePath, "spec.yaml"), d.FileMode)
}

// MoveGeneratedDirToWd performs the final step of the generation, moving the generated temp
// directory to the destination path. It tries to do this in a way where nothing is destroyed
// until everything is confirmed to work.
func (d *DistributionGenerator) MoveGeneratedDirToWd() (err error) {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	generateDest := filepath.Join(wd, d.GenerateDirName)
	bkpPath := generateDest + "-bkp"

	// Check if the distribution directory exists, rename it to backup
	// if it does.
	if _, err := os.Open(generateDest); err == nil {
		if err := os.Rename(generateDest, bkpPath); err != nil {
			return err
		}

		// Delete the backup. Sets the named `err` return value
		// if removal of backup fails.
		defer func() {
			err = os.RemoveAll(bkpPath)
		}()
	}

	// Move generated directory to working directory.
	if err := os.Rename(d.GeneratePath, generateDest); err != nil {
		return err
	}

	return nil
}

type generatedFile struct {
	path    string
	content string
	checked bool
}

var ErrDistroFolderDoesNotExist = errors.New("distribution folder not generated")
var ErrDistroFolderDiff = errors.New("existing distro folder differs from generation")

// Compare will deeply compare the newly generated distro against the existing one.
func (d *DistributionGenerator) Compare() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get working directory: %w", err)
	}

	_, err = os.Stat(d.GeneratePath)
	if err != nil {
		if os.IsNotExist(err) {
			panic(fmt.Sprintf("generated directory %s does not exist, if you see this it's a code error in distrogen", d.GeneratePath))
		}
		return wrapExitCodeError(
			unexpectErrExitCode,
			fmt.Errorf("could not stat generated directory: %w", err),
		)
	}
	generateDest := filepath.Join(wd, d.GenerateDirName)

	_, err = os.Stat(generateDest)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrDistroFolderDoesNotExist, generateDest)
		}
		return wrapExitCodeError(
			unexpectErrExitCode,
			fmt.Errorf("could not stat existing distro directory: %w", err),
		)
	}

	logger.Debug("comparing ", d.GeneratePath, generateDest)

	generatedContent, err := d.getGeneratedFilesInDir(d.GeneratePath)
	if err != nil {
		return wrapExitCodeError(
			unexpectErrExitCode,
			fmt.Errorf("could not get generated files: %w", err),
		)
	}
	existingContent, err := d.getGeneratedFilesInDir(generateDest)
	if err != nil {
		return wrapExitCodeError(
			unexpectErrExitCode,
			fmt.Errorf("could not get existing files: %w", err),
		)
	}

	errs := make(CollectionError)
	for name, existingFile := range existingContent {
		generatedFile, ok := generatedContent[name]
		if !ok {
			errs[name] = fmt.Errorf("existing file not found in generated distribution")
			continue
		}
		existingFile.checked = true
		generatedFile.checked = true
		diff := cmp.Diff(existingFile.content, generatedFile.content)
		if diff != "" {
			errs[name] = fmt.Errorf("existing file differs from generated distribution:\n%s", diff)
		}
	}

	for name, generatedFile := range generatedContent {
		if !generatedFile.checked {
			errs[name] = fmt.Errorf("generated file not found in existing distribution")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s: %w:\n\n%v", d.Spec.Name, ErrDistroFolderDiff, errs)
	}

	return nil
}

func (dg *DistributionGenerator) getGeneratedFilesInDir(dir string) (map[string]*generatedFile, error) {
	files := map[string]*generatedFile{}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Don't include .tools directory in comparison.
		if strings.Contains(path, "/.tools/") {
			return nil
		}

		if d.Name() == dg.Spec.BinaryName {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[d.Name()] = &generatedFile{
			path:    path,
			content: string(content),
		}
		return nil
	})

	return files, err
}

// Clean will remove the temporary directory used for generation.
func (d *DistributionGenerator) Clean() {
	if err := os.RemoveAll(d.GeneratePath); err != nil && !os.IsNotExist(err) {
		logger.Error("failed to clean generated directory", "err", err)
	}
}

// TemplateContext is the context that will be passed into any default or user
// provided templates.
type TemplateContext struct {
	*DistributionSpec

	Receivers  RegistryComponents
	Processors RegistryComponents
	Exporters  RegistryComponents
	Extensions RegistryComponents
	Connectors RegistryComponents
	Providers  RegistryComponents

	SystemdServiceName  string
	SystemdConfFileName string
}

// NewTemplateContextFromSpec creates a TemplateContext from a DistributionSpec and a Registry.
// It is expected that this registry will be already merged with the registries provided by the
// user.
func NewTemplateContextFromSpec(spec *DistributionSpec, registry *Registry) (*TemplateContext, error) {
	context := TemplateContext{DistributionSpec: spec}

	otelVersion := otelComponentVersion{
		core:       spec.OpenTelemetryVersion,
		coreStable: spec.OpenTelemetryStableVersion,
		contrib:    spec.OpenTelemetryContribVersion,
	}

	errs := make(CollectionError)
	var err CollectionError
	context.Receivers, err = registry.Receivers.LoadAllComponents(spec.Components.Receivers, otelVersion)
	mapMerge(errs, err)
	context.Processors, err = registry.Processors.LoadAllComponents(spec.Components.Processors, otelVersion)
	mapMerge(errs, err)
	context.Exporters, err = registry.Exporters.LoadAllComponents(spec.Components.Exporters, otelVersion)
	mapMerge(errs, err)
	context.Connectors, err = registry.Connectors.LoadAllComponents(spec.Components.Connectors, otelVersion)
	mapMerge(errs, err)
	context.Extensions, err = registry.Extensions.LoadAllComponents(spec.Components.Extensions, otelVersion)
	mapMerge(errs, err)
	context.Providers, err = registry.Providers.LoadAllComponents(spec.Components.Providers, otelVersion)
	mapMerge(errs, err)

	context.SystemdServiceName = spec.BinaryName + ".service"
	context.SystemdConfFileName = spec.BinaryName + ".conf"

	if len(errs) > 0 {
		return nil, errs
	}
	return &context, nil
}

// FeatureGates is a list of feature gate names to enable in a
// collector.
type FeatureGates []string

// Render will render the feature gates in a comma separated list.
func (fgs FeatureGates) Render() string {
	// This case should never come up in template rendering,
	// but it's here as a backup in case.
	if len(fgs) == 0 {
		return ""
	}

	gates := ""
	for i, fg := range fgs {
		gates += fg
		if i < len(fgs)-1 {
			gates += ","
		}
	}
	return gates
}

// ComponentReplace is a Go module replacement that will be
// rendered into the OCB manifest.
type ComponentReplace struct {
	From   *GoModuleID `yaml:"from"`
	To     *GoModuleID `yaml:"to"`
	Reason string      `yaml:"reason"`
}

// String renders the component replace for an OCB manifest.
func (r *ComponentReplace) String() string {
	r.From.AllowBlankTag = true
	r.To.AllowBlankTag = true
	// This is pretty awkward and it would be nice to implement yaml.Marshaler
	// on here instead, but this was the only nice way I could find to render
	// the Reason field as a comment above the replacement entry.
	return fmt.Sprintf("# %s\n- %s => %s", r.Reason, r.From, r.To)
}

// ComponentReplaces is a collection of component replacements.
type ComponentReplaces []*ComponentReplace

// Render renders the component replaces all at once
// for an OCB manifest.
func (rs ComponentReplaces) Render() string {
	result := ""
	for _, r := range rs {
		result += fmt.Sprintf("%s\n", r)
	}
	return result
}
