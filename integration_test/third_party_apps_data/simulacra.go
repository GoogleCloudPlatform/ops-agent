package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"
	"github.com/google/uuid"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
)

//go:embed applications
var scriptsDir embed.FS

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

func setupOpsAgent(ctx context.Context, vm *gce.VM, logger *logging.DirectoryLogger, configFilePath string) error {
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return err
	}

	config, err := confgenerator.UnmarshalYamlToUnifiedConfig(ctx, data)
	if err != nil {
		return err
	}

	if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, config.String()); err != nil {
		return err
	}

	return nil
}

func getAllReceivers(config *confgenerator.UnifiedConfig) (receivers []string) {
	for logs := range config.Logging.Receivers {
		app := config.Logging.Receivers[logs].Type()
		receivers = append(receivers, app)
	}

	for logs := range config.Metrics.Receivers {
		app := config.Metrics.Receivers[logs].Type()
		receivers = append(receivers, app)
	}
	return receivers
}

// installApps reads an Ops Agent config file and then identifies all the third party apps that need to be installed.
// The function identifies third party apps to install by checking if any of the receiver types have a
// corresponding install script in the third_party_apps_data directory.
// If there is a corresponding install script, then that install script is run on the vm.
func installApps(ctx context.Context, vm *gce.VM, logger *logging.DirectoryLogger, configFilePath string) error {
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
		if scriptContent, err := scriptsDir.ReadFile(path.Join("applications", app, folder, "install")); err == nil {
			logger.ToMainLog().Printf("Installing %s to VM", app)
			if _, err := gce.RunScriptRemotely(ctx, logger, vm, string(scriptContent), nil, make(map[string]string)); err != nil {
				return err
			} else {
				logger.ToMainLog().Printf("Done Installing %s", app)
			}
		}
	}
	return nil
}

func main() {
	logger, err := logging.NewDirectoryLogger(path.Join("logs"))
	if err != nil {
		log.Default().Fatalf("Failed to setup logger %v", err)
	}
	ctx := context.Background()
	platform := flag.String("platform", "debian-10", "Optional. The OS for the VM. If missing, debian-10 is used.")
	configFile := flag.String("config_file", "", "Optional. Path to the Ops Agent Config File.")
	project := flag.String("project", "", "Optional. If missing, the environment variable PROJECT will be used.")
	zone := flag.String("zone", "", "Optional. If missing, the environment variable ZONE will be used.")
	name := flag.String("name", fmt.Sprintf("simulacra-vm-instance-%s", uuid.NewString()), "Optional. A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned. ")
	flag.Parse()
	options := gce.VMOptions{
		Platform:    *platform,
		MachineType: agents.RecommendedMachineType(*platform),
		Name:        *name,
		Project:     *project,
		Zone:        *zone,
	}

	// Create VM Instance.
	vm, err := gce.CreateInstance(ctx, logger.ToFile("VM_initialization.txt"), options)
	if err != nil {
		logger.ToMainLog().Fatalf("Failed to create GCE instance %v", err)
	}

	// Install Ops Agent on VM.
	if err := setupOpsAgent(ctx, vm, logger, *configFile); err != nil {
		logger.ToMainLog().Fatalf("Failed to install Ops Agent %v", err)
	}

	// Install Third Party Appliations based on Ops Agent Config.
	if err := installApps(ctx, vm, logger, *configFile); err != nil {
		logger.ToMainLog().Fatalf("Failed to install apps %v", err)
	}

	logger.ToMainLog().Printf("VM '%s' is ready.", vm.Name)
}
