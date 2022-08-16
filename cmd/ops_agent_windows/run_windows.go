package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
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
	log          debug.Log
	userConf     string
	outDirectory string
}

func (s *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	if err := s.parseFlags(args); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to parse arguments: %v", err))
		// ERROR_INVALID_ARGUMENT
		return false, 0x00000057
	}
	if uc, err := s.generateConfigs(); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to generate config files: %v", err))
		// 2 is "file not found"
		return false, 2
	}
	s.log.Info(1, "generated configuration files")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	if err := s.startSubagents(); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to start subagents: %v", err))
		// TODO: Ignore failures for partial startup?
	}
	s.log.Info(1, "started subagents")

	metrics, err := self_metrics.CollectOpsAgentSelfMetrics(&uc)
	if err != nil {
		return false, 0
	}

	err = sendMetricsEveryIntervalWindows(metrics, r, changes)
	if err != nil {
		return err
	}

	return
}

func (s *service) parseFlags(args []string) error {
	s.log.Info(1, fmt.Sprintf("args: %#v", args))
	var fs flag.FlagSet
	fs.StringVar(&s.userConf, "in", "", "path to the user specified agent config")
	fs.StringVar(&s.outDirectory, "out", "", "directory to write generated configuration files to")

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

func (s *service) generateConfigs() (confgenerator.UnifiedConfig, error) {
	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	builtInConfig, mergedConfig, err := confgenerator.MergeConfFiles(s.userConf, "windows", apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	s.log.Info(1, fmt.Sprintf("Built-in config:\n%s", builtInConfig))
	s.log.Info(1, fmt.Sprintf("Merged config:\n%s", mergedConfig))
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(mergedConfig, "windows")
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
		if err := confgenerator.GenerateFilesFromConfig(
			&uc,
			subagent,
			filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "log"),
			filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "run"),
			filepath.Join(s.outDirectory, subagent)); err != nil {
			return err
		}
	}
	return uc, nil
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
			// TODO: Should we be ignoring failures for partial startup?
			s.log.Error(1, fmt.Sprintf("failed to start %q: %v", svc.name, err))
		}
	}
	return nil
}

func sendMetricsEveryIntervalWindows(metrics []self_metrics.IntervalMetrics, r <-chan svc.ChangeRequest, changes chan<- svc.Status) error {
	bufferChannel := make(chan []self_metrics.Metric)
	buffer := make([]self_metrics.Metric, 0)

	tickers := make([]*time.Ticker, 0)

	for _, m := range metrics {
		tickers = append(tickers, time.NewTicker(time.Duration(m.Interval)*time.Minute))
	}

	for idx, m := range metrics {
		go self_metrics.registerMetric(m, bufferChannel, tickers[idx])
	}

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
				return nil
			default:
				return fmt.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}

		case d := <-bufferChannel:
			if len(buffer) == 0 {
				go self_metrics.waitForBufferChannel(&buffer)
			}
			buffer = append(buffer, d...)
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
