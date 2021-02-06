package main

import (
	"fmt"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
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
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	handles := make([]*mgr.Service, len(services))
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
			handles[i] = serviceHandle
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
		handles[i] = serviceHandle
	}
	// Registering with the event log is required to suppress the "The description for Event ID 1 from source Google Cloud Ops Agent cannot be found" message in the logs.
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		// Ignore error since it likely means the event log already exists.
	}
	// Automatically start the Ops Agent service.
	return handles[0].Start()
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	errs := []string{}
	last := len(services) - 1
	for i := range services {
		// Have to remove the services in reverse order.
		s := services[last-i]
		serviceHandle, err := m.OpenService(s.name)
		if err != nil {
			// Service does not exist, so nothing to delete.
			continue
		}
		defer serviceHandle.Close()
		if _, err := serviceHandle.Control(windows.SERVICE_CONTROL_STOP); err != nil {
			// Don't return until all services have been processed.
			errs = append(errs, err.Error())
		}
		if err := serviceHandle.Delete(); err != nil {
			// Don't return until all services have been processed.
			errs = append(errs, err.Error())
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf(strings.Join(errs, "\n"))
	}
	return nil
}
