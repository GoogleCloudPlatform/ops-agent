package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/kardianos/osext"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func escapeExe(exepath string, args []string) string {
	// from https://github.com/golang/sys/blob/22da62e12c0c/windows/svc/mgr/mgr.go#L123
	s := syscall.EscapeArg(exepath)
	for _, v := range args {
		s += " " + syscall.EscapeArg(v)
	}
	return s
}

func install() error {
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
	// TODO: Write meaningful descriptions for these services
	services := []struct {
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
			"Google Cloud Ops Agent - Metrics Agent",
			filepath.Join(base, "google-cloud-metrics-agent_windows_amd64.exe"),
			nil,
		},
		{
			// TODO: fluent-bit hardcodes a service name of "fluent-bit"; do we need to match that?
			"Google Cloud Ops Agent - Logging Agent",
			filepath.Join(base, "fluent-bit.exe"),
			[]string{
				"-c", filepath.Join(configOutDir, `\fluentbit\fluent_bit_main.conf`),
				"-R", filepath.Join(configOutDir, `\fluentbit\fluent_bit_parser.conf`),
			},
		},
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	for i, s := range services {
		var deps []string
		if i > 0 {
			// All services depend on the config generation service.
			deps = []string{services[0].name}
		}
		serviceHandle, err := m.OpenService(s.name)
		if err == nil {
			// Service already exists; just update its configuration.
			defer serviceHandle.Close()
			config, err := serviceHandle.Config()
			if err != nil {
				return err
			}
			config.DisplayName = s.name
			config.BinaryPathName = escapeExe(s.exepath, s.args)
			config.Dependencies = deps
			if err := serviceHandle.UpdateConfig(config); err != nil {
				return err
			}
			continue
		}
		serviceHandle, err = m.CreateService(
			s.name,
			s.exepath,
			mgr.Config{DisplayName: s.name, StartType: mgr.StartAutomatic, Dependencies: deps},
			s.args...,
		)
		if err != nil {
			return err
		}
		defer serviceHandle.Close()
	}
	// Registering with the event log is required to suppress the "The description for Event ID 1 from source Google Cloud Ops Agent cannot be found" message in the logs.
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		// Ignore error since it likely means the event log already existss.
	}
	return nil
}
