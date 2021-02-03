package main

import (
	"fmt"
	"log"
	"path/filepath"
	"syscall"

	"github.com/kardianos/osext"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	if err := install(); err != nil {
		log.Fatal(err)
	}
}

func escapeExe(exepath string, args []string) string {
	// from https://github.com/golang/sys/blob/22da62e12c0c/windows/svc/mgr/mgr.go#L123
	s := syscall.EscapeArg(exepath)
	for _, v := range args {
		s += " " + syscall.EscapeArg(v)
	}
	return s
}

func install() error {
	self, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("could not determine own path: %w", err)
	}
	base, err := osext.ExecutableFolder()
	if err != nil {
		return fmt.Errorf("could not determine binary path: %w", err)
	}
	// TODO: Write meaningful descriptions for these services
	services := []struct {
		name    string
		exepath string
		args    []string
	}{
		{
			"Google Cloud Ops Agent",
			self,
			nil,
		},
		{
			"Google Cloud Ops Agent - Metrics Agent",
			filepath.Join(base, "google-cloud-metrics-agent_windows_amd64.exe"),
			nil,
		},
		{
			"Google Cloud Ops Agent - Logging Agent",
			filepath.Join(base, "fluent-bit.exe"),
			[]string{"-c", filepath.Join(base, `..\C:\dev\install1\fluentbit.conf`)},
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
	return nil
}
