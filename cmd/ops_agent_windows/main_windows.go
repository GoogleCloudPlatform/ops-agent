package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const dataDirectory = `Google/Cloud Operations/Ops Agent`
const serviceName = "google-cloud-ops-agent"
const serviceDisplayName = "Google Cloud Ops Agent"

var (
	installServices   = flag.Bool("install", false, "whether to install the services")
	uninstallServices = flag.Bool("uninstall", false, "whether to uninstall the services")
)

// Standalone agents to check for before starting.
var conflictingAgents = map[string]bool{
	"StackdriverLogging":    true,
	"StackdriverMonitoring": true,
}

func main() {
	// Fail if any standalone agents are running.
	mgr, err := mgr.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to service manager: %s", err)
	}
	services, err := mgr.ListServices()
	if err != nil {
		log.Fatalf("Failed to list services: %s", err)
	}
	var runningAgents []string
	for _, s := range services {
		if conflictingAgents[s] {
			runningAgents = append(runningAgents, s)
		}
	}
	if len(runningAgents) > 0 {
		log.Fatalf("Detected the following Google agents already running: %v.  "+
			"The Ops Agent is not compatible with those agents.  Please "+
			"consult the documentation to save the configuration of the "+
			"existing agents and disable them, then retry enabling the "+
			"Ops Agent", runningAgents)
	}

	if ok, err := svc.IsWindowsService(); ok && err == nil {
		if err := run(serviceName); err != nil {
			log.Fatal(err)
		}
	} else if err != nil {
		log.Fatalf("failed to talk to service control manager: %v", err)
	} else {
		flag.Parse()
		if *installServices && *uninstallServices {
			log.Fatal("Can't use both --install and --uninstall")
		}
		if *installServices {
			if err := install(); err != nil {
				log.Fatal(err)
			}
			log.Printf("installed services")
		} else if *uninstallServices {
			if err := uninstall(); err != nil {
				log.Fatal(err)
			}
			log.Printf("uninstalled services")
		} else {
			// TODO: add an interactive GUI box with the Install, Uninstall, and Cancel buttons.
			fmt.Println("Invoked as a standalone program with no flags. Nothing to do.")
			fmt.Println("Use either --install or --uninstall to take action.")
		}
	}
}

var services []struct {
	name        string
	displayName string
	exepath     string
	args        []string
}

func init() {
	if err := initServices(); err != nil {
		log.Fatal(err)
	}
}

func initServices() error {
	// Identify relevant paths
	self, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("could not determine own path: %w", err)
	}
	base, err := osext.ExecutableFolder()
	if err != nil {
		return fmt.Errorf("could not determine binary path: %w", err)
	}
	configOutDir := filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "generated_configs")
	if err := os.MkdirAll(configOutDir, 0644); err != nil {
		return err
	}
	fluentbitStoragePath := filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, `run\buffers`)
	if err := os.MkdirAll(fluentbitStoragePath, 0644); err != nil {
		return err
	}
	logDirectory := filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "log")
	if err := os.MkdirAll(logDirectory, 0644); err != nil {
		return err
	}
	// TODO: Write meaningful descriptions for these services
	services = []struct {
		name        string
		displayName string
		exepath     string
		args        []string
	}{
		{
			serviceName,
			serviceDisplayName,
			self,
			[]string{"-in", filepath.Join(base, "../config/config.yaml"), "-out", configOutDir},
		},
		{
			fmt.Sprintf("%s-opentelemetry-collector", serviceName),
			fmt.Sprintf("%s - Metrics Agent", serviceDisplayName),
			filepath.Join(base, "google-cloud-metrics-agent_windows_amd64.exe"),
			[]string{
				"--add-instance-id=false",
				"--config=" + filepath.Join(configOutDir, `otel\otel.yaml`),
			},
		},
		{
			// TODO: fluent-bit hardcodes a service name of "fluent-bit"; do we need to match that?
			fmt.Sprintf("%s-fluent-bit", serviceName),
			fmt.Sprintf("%s - Logging Agent", serviceDisplayName),
			filepath.Join(base, "fluent-bit.exe"),
			[]string{
				"-c", filepath.Join(configOutDir, `fluentbit\fluent_bit_main.conf`),
				"-R", filepath.Join(configOutDir, `fluentbit\fluent_bit_parser.conf`),
				"--storage_path", fluentbitStoragePath,
				"--log_file", filepath.Join(logDirectory, "logging-module.log"),
			},
		},
	}
	return nil
}
