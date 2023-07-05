package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/binxio/gcloudconfig"
	"github.com/google/uuid"
)

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
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return err
	}

	config, err := confgenerator.UnmarshalYamlToUnifiedConfig(ctx, data)
	if err != nil {
		return err
	}

	if err := agents.SetupOpsAgent(ctx, logger, vm, config.String()); err != nil {
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

// Note: The following functions are mostly a duplicate of helper functions
// that already exist in gce_testing.go. The reason why we have them here is so that we
// can use gce.RunRemotely to execute our script. gce.RunScriptRemotely does this for us
// but it expects a Directory Logger. A directory logger does not make much sense for our
// purposes.

// envVarMapToBashPrefix converts a map of env variable name to value into a string
// suitable for passing to bash as a way to set those variables. The environment values
// are wrapped in quotes. Example output: `VAR1='foo' VAR2='bar' `
func envVarMapToBashPrefix(env map[string]string) string {
	var builder strings.Builder
	for key, value := range env {
		fmt.Fprintf(&builder, "%s='%s' ", key, value)
	}
	return builder.String()
}

// envVarMapToPowershellPrefix converts a map of env variable name to value into a string
// suitable for prepending onto a powershell command as a way to set those variables.
// Example output: "$env:VAR1='foo'\n$env:VAR2='bar'\n"
func envVarMapToPowershellPrefix(env map[string]string) string {
	var builder strings.Builder
	for key, value := range env {
		fmt.Fprintf(&builder, "$env:%s='%s'\n", key, value)
	}
	return builder.String()
}

func runScriptRemotely(ctx context.Context, logger *log.Logger, vm *gce.VM, scriptContents string, env map[string]string) (_ gce.CommandOutput, err error) {
	if gce.IsWindows(vm.Platform) {
		// Use a UUID for the script name in case RunScriptRemotely is being
		// called concurrently on the same VM.
		scriptPath := "C:\\" + uuid.NewString() + ".ps1"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(scriptContents), scriptPath); err != nil {
			return gce.CommandOutput{}, err
		}
		// powershell -File seems to drop certain kinds of errors:
		// https://stackoverflow.com/a/15779295
		// In testing, adding $ErrorActionPreference = 'Stop' to the start of each
		// script seems to work around this completely.
		return gce.RunRemotely(ctx, logger, vm, "", envVarMapToPowershellPrefix(env)+"powershell -File "+scriptPath)
	}
	scriptPath := uuid.NewString() + ".sh"
	// Write the script contents to <UUID>.sh, then tell bash to execute it with -x
	// to print each line as it runs.
	// Use a UUID for the script name in case RunScriptRemotely is being called
	// concurrently on the same VM.
	return gce.RunRemotely(ctx, logger, vm, scriptContents, "cat - > "+scriptPath+" && sudo "+envVarMapToBashPrefix(env)+"bash -x "+scriptPath)
}

// installApps reads an Ops Agent config file and then identifies all the third party apps that need to be installed.
// The function identifies third party apps to install by checking if any of the receiver types have a
// corresponding install script in the third_party_apps_data directory.
// If there is a corresponding install script, then that install script is run on the vm.
func installApps(ctx context.Context, vm *gce.VM, logger *log.Logger, configFilePath string, installPath string) error {
	config, err := confgenerator.MergeConfFiles(ctx, configFilePath, apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	folder, err := distroFolder(vm.Platform)

	if err != nil {
		return err
	}

	receivers := getAllReceivers(config)

	for _, app := range receivers {
		if scriptContent, err := os.ReadFile(path.Join(installPath, "applications", app, folder, "install")); err == nil {
			logger.Printf("Installing %s to VM", app)
			if _, err := runScriptRemotely(ctx, logger, vm, string(scriptContent), make(map[string]string)); err != nil {
				return err
			} else {
				logger.Printf("Done Installing %s", app)
			}
		}
	}
	return nil
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

func main() {
	logger := log.Default()
	ctx := context.Background()
	platform := flag.String("platform", "debian-11", "Optional. The OS for the VM. If missing, debian-11 is used.")
	configFile := flag.String("config_file", "", "Optional. Path to the Ops Agent Config File.")
	project := flag.String("project", "", "Optional. If missing, the environment variable PROJECT will be used.")
	zone := flag.String("zone", "", "Optional. If missing, the environment variable ZONE will be used.")
	name := flag.String("name", fmt.Sprintf("simulacra-vm-instance-%s", uuid.NewString()), "Optional. A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned. ")
	thirdPartyAppsPath := flag.String("install_path", "./integration_test/third_party_apps_data", "Optional. The path to the third party apps data folder. If missing, Simulacra assumes the working directory is the root of the repo. Therefore, the default path is './integration_test/third_party_apps_data' ")
	flag.Parse()

	if err := configureFromGCloud(project, zone); err != nil {
		log.Fatalf("project and zone must either be non empty or set in GCloud %v", err)
	}

	options := gce.VMOptions{
		Platform:    *platform,
		MachineType: agents.RecommendedMachineType(*platform),
		Name:        *name,
		Project:     *project,
		Zone:        *zone,
	}
	// Create VM Instance.
	vm, err := gce.CreateInstance(ctx, logger, options)
	if err != nil {
		logger.Fatalf("Failed to create GCE instance %v", err)
	}
	// Install Ops Agent on VM.
	if err := setupOpsAgent(ctx, vm, logger, *configFile); err != nil {
		logger.Fatalf("Failed to install Ops Agent %v", err)
	}

	// Install Third Party Appliations based on Ops Agent Config.
	if err := installApps(ctx, vm, logger, *configFile, *thirdPartyAppsPath); err != nil {
		logger.Fatalf("Failed to install apps %v", err)
	}

	logger.Printf("VM '%s' is ready.", vm.Name)

}
