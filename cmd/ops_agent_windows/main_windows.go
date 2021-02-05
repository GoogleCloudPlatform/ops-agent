package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
	"golang.org/x/sys/windows/svc"
)

const dataDirectory = `Google/Cloud Operations/Ops Agent`
const serviceName = "Google Cloud Ops Agent"

func main() {
	if ok, err := svc.IsWindowsService(); ok && err == nil {
		if err := run(serviceName); err != nil {
			log.Fatal(err)
		}
	} else if err != nil {
		log.Fatalf("failed to talk to service control manager: %v", err)
	} else {
		if err := install(); err != nil {
			log.Fatal(err)
		}
		log.Printf("installed services")
	}
}

var services []struct {
	name    string
	exepath string
	args    []string
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
		name    string
		exepath string
		args    []string
	}{
		{
			serviceName,
			self,
			[]string{"-in", filepath.Join(base, "../config/config.yaml"), "-out", configOutDir},
		},
		{
			fmt.Sprintf("%s - Metrics Agent", serviceName),
			filepath.Join(base, "google-cloud-metrics-agent_windows_amd64.exe"),
			[]string{
				"--add-instance-id=false",
				"--config=" + filepath.Join(configOutDir, `otel\otel.yaml`),
			},
		},
		{
			// TODO: fluent-bit hardcodes a service name of "fluent-bit"; do we need to match that?
			fmt.Sprintf("%s - Logging Agent", serviceName),
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
