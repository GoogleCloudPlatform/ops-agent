package main

import (
	"syscall"

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
		// Ignore error since it likely means the event log already existss.
	}
	// Automatically start the Ops Agent service
	return handles[0].Start()
}
