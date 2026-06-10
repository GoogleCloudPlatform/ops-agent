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
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/cmd/distrogen/internal/command"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var (
	logLevel            = new(slog.LevelVar)
	logger              = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	unexpectErrExitCode = 2

	errNoSpecFlag = errors.New("missing --spec flag")
)

func main() {
	runner := command.NewRunner()

	runner.Register("generate", &generateCommand{})
	runner.Register("query", &queryCommand{})
	runner.Register("otel_component_versions", &otelComponentVersionsCommand{})
	runner.Register("project", &projectCommand{})
	runner.Register("component", &componentCommand{})
	runner.Register("registry", &registryCommand{})

	detectVerboseFlag()

	var exitCodeErr *ExitCodeError
	if err := runner.Run(os.Args[1]); err != nil {
		if errors.Is(err, ErrNoDiff) {
			// No diff means we just want to log the error
			// but not exit with code 1.
			log.Println(err)
		} else if errors.As(err, &exitCodeErr) {
			logger.Error(fmt.Sprintf("unexpected error: %v", err))
			os.Exit(exitCodeErr.exitCode)
		} else {
			log.Fatal(err)
		}
	}
}

func detectVerboseFlag() {
	verboseArg := slices.Index(os.Args, "--verbose")
	vArg := slices.Index(os.Args, "-v")
	if vArg != -1 {
		logLevel.Set(slog.LevelDebug)
		os.Args = slices.Delete(os.Args, vArg, vArg+1)
	}
	if verboseArg != -1 {
		logLevel.Set(slog.LevelDebug)
		os.Args = slices.Delete(os.Args, verboseArg, verboseArg+1)
	}
}

func setSpecFlag(flags *flag.FlagSet) *string {
	return flags.String("spec", "", "The distribution specification to use")
}

type generateCommand struct {
	flags flag.FlagSet

	registries *[]string
	spec       *string
	force      *bool
	templates  *string
	compare    *bool
}

func (cmd *generateCommand) ParseArgs(args []string) error {
	cmd.spec = setSpecFlag(&cmd.flags)
	cmd.force = cmd.flags.BoolP("force", "f", false, "Force generate even if there are no differences detected")
	cmd.registries = cmd.flags.StringArray("registry", []string{}, "Provide additional component registries")
	cmd.templates = cmd.flags.String("templates", "", "Path to custom templates directory")
	cmd.compare = cmd.flags.Bool("compare", false, "Allows you to compare the generated distribution to the existing")

	return cmd.flags.Parse(args)
}

func (cmd *generateCommand) Run() error {
	if *cmd.spec == "" {
		return errNoSpecFlag
	}

	spec, err := NewDistributionSpec(*cmd.spec)
	if err != nil {
		return err
	}

	registry, err := LoadEmbeddedRegistry()
	if err != nil {
		return err
	}

	for _, registryPath := range *cmd.registries {
		additionalRegistry, err := LoadRegistry(registryPath)
		if err != nil {
			return err
		}
		registry.Merge(additionalRegistry)
	}

	generator, err := NewDistributionGenerator(spec, registry, *cmd.force)
	if err != nil {
		return err
	}
	defer generator.Clean()

	if *cmd.templates != "" {
		generator.CustomTemplatesDir = os.DirFS(*cmd.templates)
	}

	if err := generator.Generate(); err != nil {
		return err
	}

	if *cmd.compare {
		return generator.Compare()
	}

	return generator.MoveGeneratedDirToWd()
}

type queryCommand struct {
	flags flag.FlagSet

	spec  *string
	field *string
}

func (cmd *queryCommand) ParseArgs(args []string) error {
	cmd.spec = setSpecFlag(&cmd.flags)
	cmd.field = cmd.flags.String("field", "", "Field to query from the spec")

	return cmd.flags.Parse(args)
}

func (cmd *queryCommand) Run() error {
	if *cmd.spec == "" {
		return errNoSpecFlag
	}

	spec, err := NewDistributionSpec(*cmd.spec)
	if err != nil {
		return err
	}

	val, err := spec.Query(*cmd.field)
	if err != nil {
		return err
	}

	// Using Println instead of logger since the results
	// may be piped to another program.
	fmt.Println(val)
	return nil
}

type otelComponentVersionsCommand struct {
	flags flag.FlagSet

	otelVersion *string
}

func (cmd *otelComponentVersionsCommand) ParseArgs(args []string) error {
	cmd.otelVersion = cmd.flags.String("otel_version", "", "The OpenTelemetry version to fetch component versions for")

	return cmd.flags.Parse(args)
}

func (cmd *otelComponentVersionsCommand) Run() error {
	type moduleSet struct {
		Version string   `yaml:"version"`
		Modules []string `yaml:"modules"`
	}
	type versions struct {
		ModuleSets      map[string]moduleSet `yaml:"module-sets"`
		ExcludedModules []string             `yaml:"excluded-modules"`
	}

	if *cmd.otelVersion == "" {
		return fmt.Errorf("otel_version flag is required")
	}

	s := bufio.NewScanner(os.Stdin)
	var allModules []string
	for s.Scan() {
		allModules = append(allModules, s.Text())
	}
	if err := s.Err(); err != nil {
		return err
	}

	response, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector/refs/tags/%s/versions.yaml", *cmd.otelVersion))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	var componentVersions versions
	if err := yaml.Unmarshal(content, &componentVersions); err != nil {
		return err
	}

	for _, moduleSet := range componentVersions.ModuleSets {
		for _, module := range allModules {
			if slices.Contains(moduleSet.Modules, module) {
				fmt.Printf("%s@%s\n", module, moduleSet.Version)
			}
		}
	}

	return nil
}

type projectCommand struct {
	flags flag.FlagSet
	tools *[]string
	spec  *string
}

func (cmd *projectCommand) ParseArgs(args []string) error {
	cmd.spec = setSpecFlag(&cmd.flags)
	cmd.tools = cmd.flags.StringArray("tools", []string{}, "Provide additional tools to install")

	return cmd.flags.Parse(args)
}

func (cmd *projectCommand) Run() error {
	if *cmd.spec == "" {
		return errNoSpecFlag
	}

	spec, err := NewDistributionSpec(*cmd.spec)
	if err != nil {
		return err
	}

	generator, err := NewProjectGenerator(spec)
	if err != nil {
		return err
	}

	return generator.Generate()
}

type componentCommand struct {
	flags flag.FlagSet

	spec          *string
	componentType *string
	componentName *string
}

func (cmd *componentCommand) ParseArgs(args []string) error {
	cmd.spec = setSpecFlag(&cmd.flags)
	cmd.componentType = cmd.flags.String("type", "", "Type of component")
	cmd.componentName = cmd.flags.String("name", "", "Name of component")

	return cmd.flags.Parse(args)
}

func (cmd *componentCommand) Run() error {
	if *cmd.spec == "" {
		return errNoSpecFlag
	}
	if *cmd.componentType == "" {
		return errors.New("missing --type flag")
	}
	if *cmd.componentName == "" {
		return errors.New("missing --name flag")
	}

	spec, err := NewDistributionSpec(*cmd.spec)
	if err != nil {
		return err
	}

	generator, err := NewComponentGenerator(spec, ComponentType(*cmd.componentType), *cmd.componentName)
	if err != nil {
		return err
	}

	return generator.Generate()
}

type registryCommand struct {
}

func (cmd *registryCommand) ParseArgs(args []string) error {
	return nil
}

func (cmd *registryCommand) Run() error {
	generator := NewComponentsRegistryGenerator()

	return generator.Generate()
}
