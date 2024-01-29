// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration_test

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"
	"github.com/binxio/gcloudconfig"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	defaultImageFamily        = "debian-11"
	defaultThirdPartyAppsPath = "./integration_test/third_party_apps_data"
	vmInitLogFileName         = "vm_initialization.txt"
)

// Script represents each individual item of the 'scripts' field in a Simulacra config. Each script in the scripts field
// will be executed on the VM once the VM instantiates.
type Script struct {
	// The path to the script file.
	Path string `yaml:"path"`
	// Command line arguments that the script will be executed with. For example, setting 'args' to ["-Apps","all"] will result in
	// Simulacra executing the script as follows : "./script -Apps all".
	Args []string `yaml:"args"`
}

// Config represents the configuration for Simulacra. Most of the fields specify requirements about the VM that
// Simulacra will instantiate.
type Config struct {
	// The image family of the OS that the VM is using.
	ImageFamily string `yaml:"image_family"`
	// The exact image that the OS is using.
	Image string `yaml:"image"`
	// Path to the Ops Agent Config File.
	ConfigFilePath string `yaml:"ops_agent_config"`
	// The Project Simulacra will be using to instantiate the VM.
	Project string `yaml:"project"`
	// Zone for the VM.
	Zone string `yaml:"zone"`
	// Name for the VM.
	Name string `yaml:"name"`
	// Path to Third Party Apps folder
	ThirdPartyAppsPath string `yaml:"third_party_apps_path"`
	// Path to script files that will be run on the VM.
	Scripts []*Script `yaml:"scripts"`
	// A Service Account for the VM.
	ServiceAccount string `yaml:"service_account"`
	// Path to a directory containing the output from the diagnostic tool.
	DiagnosticOutputPath string `yaml:"diagnostic_output_path"`
	// Passed with the --image/--image-family arguments. If unspecified, default value is 'global'
	ImageFamilyScope string `yaml:"image_family_scope"`
	// The project that the image belongs to.
	ImageProject string
}

func distroFolder(platform string) (string, error) {
	if gce.IsWindows(platform) {
		return "windows", nil
	}
	firstWord := strings.Split(platform, "-")[0]
	switch firstWord {
	case "centos", "rhel", "rocky":
		return "centos_rhel", nil
	case "debian", "ubuntu":
		return "debian_ubuntu", nil
	case "opensuse", "sles":
		return "sles", nil
	}
	return "", fmt.Errorf("distroFolder() could not find matching folder holding scripts for platform %s", platform)
}

func setupOpsAgent(ctx context.Context, vm *gce.VM, logger *log.Logger, configFilePath string) error {
	var configString string
	if configFilePath != "" {
		data, err := os.ReadFile(configFilePath)
		if err != nil {
			return err
		}

		config, err := confgenerator.UnmarshalYamlToUnifiedConfig(ctx, data)
		if err != nil {
			return err
		}

		configString = config.String()
	}

	if err := agents.SetupOpsAgent(ctx, logger, vm, configString); err != nil {
		return err
	}

	return nil
}

func getAllReceivers(config *confgenerator.UnifiedConfig) (receivers []string) {
	for _, receiver := range config.Logging.Receivers {
		receivers = append(receivers, receiver.Type())
	}

	for _, receiver := range config.Metrics.Receivers {
		receivers = append(receivers, receiver.Type())
	}
	return receivers
}

// installApps takes in a list of receivers that the Ops Agent is configured with and installs all third party apps.
// The function determines if a receiver requires a third party app installation if there is a corresponding install
// script in the third party apps data folder whose path is specified using the installPath argument.
func installApps(ctx context.Context, vm *gce.VM, logger *logging.DirectoryLogger, installPath string, receivers []string) error {
	folder, err := distroFolder(vm.Platform)

	if err != nil {
		return err
	}

	for _, app := range receivers {
		if scriptContent, err := os.ReadFile(filepath.Join(installPath, "applications", app, folder, "install")); err == nil {
			logger.ToMainLog().Printf("Installing %s to VM", app)
			log.Default().Printf("Installing %s to VM", app)
			if _, err := gce.RunScriptRemotely(ctx, logger.ToMainLog(), vm, string(scriptContent), nil, make(map[string]string)); err != nil {
				return fmt.Errorf("Failed to install app %s %v", app, err)
			}
			logger.ToMainLog().Printf("Done Installing %s", app)
			log.Default().Printf("Done Installing %s", app)

		}
	}
	return nil
}

func getReceiversFromConfig(ctx context.Context, vm *gce.VM, logger *logging.DirectoryLogger, configFilePath string) ([]string, error) {
	if configFilePath == "" {
		return []string{}, nil
	}

	config, err := confgenerator.MergeConfFiles(ctx, configFilePath, apps.BuiltInConfStructs)
	if err != nil {
		return nil, err
	}

	receivers := getAllReceivers(config)
	return receivers, nil
}

func configureFromGCloud(project *string, zone *string) error {
	config, err := gcloudconfig.GetConfig("")
	if err != nil && (*project == "" || *zone == "") {
		return err
	}

	if *project == "" {
		*project = *config.Configuration.Properties.Core.Project
	}

	if *zone == "" {
		*zone = *config.Configuration.Properties.Compute.Zone
	}

	return nil

}

func getInstanceName() string {
	return fmt.Sprintf("simulacra-vm-instance-%s", uuid.NewString())
}

func getConfigFromYaml(configPath string) (*Config, error) {
	var config Config
	file, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(file, &config); err != nil {
		return nil, err
	}

	if config.ImageFamily == "" && config.Image == "" {
		config.ImageFamily = defaultImageFamily
	}

	if config.ThirdPartyAppsPath == "" {
		config.ThirdPartyAppsPath = defaultThirdPartyAppsPath
	}

	if config.Name == "" {
		config.Name = getInstanceName()
	}

	return &config, nil
}

// Parse the metadata image name with format 'projects/debian-cloud/global/images/debian-11-bullseye-v20230711'
// and return the scope and image name.
func parseImageFromMetadata(name string) (string, string, string, error) {
	components := strings.Split(name, "/")
	if len(components) < 5 {
		return "", "", "", errors.New("image name from metadata must be of format 'projects/debian-cloud/global/images/debian-11-bullseye-v20230711' ")
	}
	imgProject := components[1]
	scope := components[2]
	image := components[4]
	return imgProject, scope, image, nil
}

// Returns the image project, scope, image and image family.
// If the image name in the metadata file starts with the
// prefix, "/projects", the value is parsed for the project, scope, image.
// Otherwise, we ask for the image family from the user using command line input.
func getImageInfo(name string) (string, string, string, string, error) {
	if strings.HasPrefix(name, "projects/") {
		imgProject, scope, image, err := parseImageFromMetadata(name)
		return imgProject, scope, image, "", err
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Unable to identify image family, Enter Image Family: ")
	text, err := reader.ReadString('\n')
	return "", "", "", text, err

}

func getConfigFromDiagnosticOutput(outputDir string) (*Config, error) {
	type Metadata struct {
		Image string `json:"image"`
	}

	metadataFile, err := os.ReadFile(filepath.Join(outputDir, "vm_config.json"))
	if err != nil {
		return nil, err
	}

	var metadata Metadata
	if err := json.Unmarshal(metadataFile, &metadata); err != nil {
		return nil, err
	}

	imgProject, scope, image, imgFamily, err := getImageInfo(metadata.Image)

	if err != nil {
		return nil, err
	}

	config := &Config{
		Image:              image,
		Name:               getInstanceName(),
		ImageFamilyScope:   scope,
		ThirdPartyAppsPath: defaultThirdPartyAppsPath,
		ImageProject:       imgProject,
		ImageFamily:        imgFamily,
	}

	configFilePath := filepath.Join(outputDir, "google-cloud-ops-agent", "config.yaml")
	if _, err := os.Stat(configFilePath); err == nil {
		config.ConfigFilePath = configFilePath
	}

	return config, nil

}

func getSimulacraConfig() (*Config, error) {
	configPath := flag.String("config", "", "Optional. The path to a YAML file specifying all the configurations for Simulacra. If unspecified, Simulacra will either use values from other command line arguments or use default values. If specifed along with other command line arguments, all others will be ignored.")
	diagnosticOutputPath := flag.String("diagnostic_output_path", "", "Optional. The path to a directory contaning the output from the ops agent diagnostic tool. If specified, all other arguments will be ignored and Simulacra will be configured from the diagnostic tool output.")
	imageFamily := flag.String("image_family", defaultImageFamily, "Optional. The OS for the VM. If missing, debian-11 is used.")
	opsAgentConfigFile := flag.String("ops_agent_config", "", "Optional. Path to the Ops Agent Config File. If unspecified, Ops Agent will not install any third party applications and configure Ops Agent with default settings. ")
	project := flag.String("project", "", "Optional. If missing, Simulacra will try to infer from GCloud config.")
	zone := flag.String("zone", "", "Optional. If missing, Simulacra will try to infer from GCloud config. ")
	name := flag.String("name", getInstanceName(), "Optional. A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned. ")
	thirdPartyAppsPath := flag.String("third_party_apps_path", defaultThirdPartyAppsPath, "Optional. The path to the third party apps data folder. If missing, Simulacra assumes the working directory is the root of the repo. Therefore, the default path is './integration_test/third_party_apps_data' ")
	serviceAccount := flag.String("service_account", "", "Optional. A service account for the VM. If missing, the VM will be instantiated with a default service account.")
	flag.Parse()

	if *configPath != "" {
		return getConfigFromYaml(*configPath)
	}

	if *diagnosticOutputPath != "" {
		return getConfigFromDiagnosticOutput(*diagnosticOutputPath)
	}

	config := Config{
		ImageFamily:        *imageFamily,
		ConfigFilePath:     *opsAgentConfigFile,
		Project:            *project,
		Zone:               *zone,
		Name:               *name,
		ThirdPartyAppsPath: *thirdPartyAppsPath,
		ServiceAccount:     *serviceAccount,
	}

	return &config, nil

}

func runCustomScripts(ctx context.Context, vm *gce.VM, logger *logging.DirectoryLogger, scripts []*Script) error {
	for _, script := range scripts {
		scriptContent, err := os.ReadFile(script.Path)

		if err != nil {
			return err
		}

		logger.ToMainLog().Printf("Running script from %s", script.Path)
		log.Default().Printf("Running script from %s", script.Path)
		if _, err := gce.RunScriptRemotely(ctx, logger.ToMainLog(), vm, string(scriptContent), script.Args, make(map[string]string)); err != nil {
			return fmt.Errorf("Script with path %s failed to run %v", script.Path, err)
		}
		logger.ToMainLog().Printf("Done Running Script from  %s", script.Path)

	}

	return nil
}

func getRecommendedMachineType(imageFamily string, image string) string {
	if imageFamily != "" {
		return agents.RecommendedMachineType(imageFamily)
	}

	return agents.RecommendedMachineType(image)
}

func createInstance(ctx context.Context, config *Config, logger *log.Logger) (*gce.VM, error) {
	args := []string{}
	if config.ServiceAccount != "" {
		args = append(args, "--service-account="+config.ServiceAccount)
	}

	options := gce.VMOptions{
		Platform: config.ImageFamily,
		// TODO: Revert this setting once the default in gce_testing.go is
		// changed to be infinite.
		TimeToLive:           "365d",
		Image:                config.Image,
		ImageFamilyScope:     config.ImageFamilyScope,
		ImageProject:         config.ImageProject,
		MachineType:          getRecommendedMachineType(config.ImageFamily, config.Image),
		Name:                 config.Name,
		Project:              config.Project,
		Zone:                 config.Zone,
		ExtraCreateArguments: args,
	}

	return gce.CreateInstance(ctx, logger, options)
}

func main() {
	loggingDir := filepath.Join("/tmp", fmt.Sprintf("simulacra-%s", uuid.NewString()))
	mainLogFile := filepath.Join(loggingDir, "main_log.txt")
	vmInitLogFile := filepath.Join(loggingDir, vmInitLogFileName)
	logger, err := logging.NewDirectoryLogger(loggingDir)
	if err != nil {
		log.Default().Fatalf("Error initializing directory logger %v", err)
	}
	log.Default().Printf("Starting Simulacra, Detailed logging can be found in %s directory", loggingDir)
	ctx := context.Background()
	config, err := getSimulacraConfig()
	if err != nil {
		log.Default().Fatalf("error parsing simulacra config %v", err)
	}

	if err := configureFromGCloud(&config.Project, &config.Zone); err != nil {
		log.Default().Fatalf("project and zone must either be non empty or set in GCloud %v", err)
	}

	// Create VM Instance.
	log.Default().Printf("Creating VM Instance, check %s for details", vmInitLogFile)
	vm, err := createInstance(ctx, config, logger.ToFile(vmInitLogFileName))
	if err != nil {
		log.Default().Fatalf("Failed to create GCE instance %v", err)
	}
	// Install Ops Agent on VM.
	log.Default().Printf("Installing Ops Agent, check %s for details", mainLogFile)
	if err := setupOpsAgent(ctx, vm, logger.ToMainLog(), config.ConfigFilePath); err != nil {
		log.Default().Fatalf("Failed to install Ops Agent %v", err)
	}

	// Install Third Party Appliations based on Ops Agent Config.
	log.Default().Printf("Installing Third Party Applications, check %s for details", mainLogFile)
	receivers, err := getReceiversFromConfig(ctx, vm, logger, config.ConfigFilePath)

	if err != nil {
		log.Default().Fatalf("Error reading config file: %v", err)
	}

	if err := installApps(ctx, vm, logger, config.ThirdPartyAppsPath, receivers); err != nil {
		log.Default().Printf("Failed to install apps %v", err)
	}

	// Run custom Scripts on the VM

	log.Default().Printf("Running Custom Scripts on the VM, check %s for details", mainLogFile)
	if err := runCustomScripts(ctx, vm, logger, config.Scripts); err != nil {
		log.Default().Fatalf("Error executing custom script on the VM %v", err)
	}

	log.Default().Printf("VM '%s' is ready.", vm.Name)
	logger.ToMainLog().Printf("VM '%s' is ready", vm.Name)
	logger.Close()

}
