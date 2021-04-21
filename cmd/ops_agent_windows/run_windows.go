package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func containsString(all []string, s string) bool {
	for _, t := range all {
		if t == s {
			return true
		}
	}
	return false
}

type service struct {
	log                  debug.Log
	inFile, outDirectory string
}

func (s *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	if err := s.parseFlags(args); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to parse arguments: %v", err))
		// ERROR_INVALID_ARGUMENT
		return false, 0x00000057
	}
	if err := s.generateConfigs(); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to generate config files: %v", err))
		// 2 is "file not found"
		return false, 2
	}
	s.log.Info(1, "generated configuration files")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	if err := s.startSubagents(); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to start subagents: %v", err))
		// ERROR_SERVICE_DEPENDENCY_FAIL
		return false, 0x0000042C
	}
	s.log.Info(1, "started subagents")
	defer func() {
		changes <- svc.Status{State: svc.StopPending}
	}()
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				return
			default:
				s.log.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}
		}
	}
	return
}

func (s *service) parseFlags(args []string) error {
	s.log.Info(1, fmt.Sprintf("args: %#v", args))
	var fs flag.FlagSet
	fs.StringVar(&s.inFile, "in", "", "input filename")
	fs.StringVar(&s.outDirectory, "out", "", "output directory")
	allArgs := append([]string{}, os.Args[1:]...)
	allArgs = append(allArgs, args[1:]...)
	return fs.Parse(allArgs)
}

func (s *service) checkForStandaloneAgents(unified *confgenerator.UnifiedConfig) error {
	mgr, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %s", err)
	}
	defer mgr.Disconnect()
	services, err := mgr.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services: %s", err)
	}

	var errors string
	if unified.HasLogging() && containsString(services, "StackdriverLogging") {
		errors += "We detected an existing Windows service for the StackdriverLogging agent, " +
			"which is not compatible with the Ops Agent when the Ops Agent configuration has a non-empty logging section. " +
			"Please either remove the logging section from the Ops Agent configuration, " +
			"or disable the StackdriverLogging agent, and then retry enabling the Ops Agent. "
	}
	if unified.HasMetrics() && containsString(services, "StackdriverMonitoring") {
		errors += "We detected an existing Windows service for the StackdriverMonitoring agent, " +
			"which is not compatible with the Ops Agent when the Ops Agent configuration has a non-empty metrics section. " +
			"Please either remove the metrics section from the Ops Agent configuration, " +
			"or disable the StackdriverMonitoring agent, and then retry enabling the Ops Agent. "
	}
	if errors != "" {
		return fmt.Errorf("conflicts with existing agents: %s", errors)
	}
	return nil
}

func (s *service) generateConfigs() error {
	data, err := ioutil.ReadFile(s.inFile)
	if err != nil {
		return err
	}
	uc, err := confgenerator.ParseUnifiedConfig(data)
	if err != nil {
		return err
	}
	if err := s.checkForStandaloneAgents(&uc); err != nil {
		return err
	}
	// TODO: Add flag for passing in log/run path?
	for _, subagent := range []string{
		"otel",
		"fluentbit",
	} {
		if err := uc.GenerateFiles(
			subagent,
			filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "log"),
			filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "run"),
			filepath.Join(s.outDirectory, subagent)); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) startSubagents() error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	for _, svc := range services[1:] {
		handle, err := manager.OpenService(svc.name)
		if err != nil {
			// service not found?
			return err
		}
		defer handle.Close()
		if err := handle.Start(); err != nil {
			return fmt.Errorf("failed to start %q: %v", svc.name, err)
		}
	}
	return nil
}

func run(name string) error {
	elog, err := eventlog.Open(name)
	if err != nil {
		// probably futile
		return err
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("starting %s service", name))
	err = svc.Run(name, &service{log: elog})
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", name, err))
		return err
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", name))
	return nil
}
