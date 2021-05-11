package confgenerator

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/shirou/gopsutil/host"
)

func GenerateFiles(input, service, logsDir, stateDir, outDir string) error {
	data, err := ioutil.ReadFile(input)
	if err != nil {
		return err
	}
	uc, err := ParseUnifiedConfig(data)
	if err != nil {
		return err
	}
	return uc.GenerateFiles(service, logsDir, stateDir, outDir)
}

func (uc *UnifiedConfig) GenerateFiles(service, logsDir, stateDir, outDir string) error {
	hostInfo, _ := host.Info()
	switch service {
	case "fluentbit":
		mainConfig, parserConfig, err := uc.GenerateFluentBitConfigs(logsDir, stateDir, hostInfo)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		// Make sure the output directory exists before generating configs.
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("can't create output directory %q: %w", outDir, err)
		}
		path := filepath.Join(outDir, "fluent_bit_main.conf")
		if err := ioutil.WriteFile(path, []byte(mainConfig), 0644); err != nil {
			return fmt.Errorf("can't write %q: %w", path, err)
		}
		path = filepath.Join(outDir, "fluent_bit_parser.conf")
		if err := ioutil.WriteFile(path, []byte(parserConfig), 0644); err != nil {
			return fmt.Errorf("can't write %q: %w", path, err)
		}
	case "collectd":
		collectdConfig, err := uc.GenerateCollectdConfig(logsDir)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		// Make sure the output directory exists before generating configs.
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("can't create output directory %q: %w", outDir, err)
		}
		path := filepath.Join(outDir, "collectd.conf")
		if err := ioutil.WriteFile(path, []byte(collectdConfig), 0644); err != nil {
			return fmt.Errorf("can't write %q: %w", path, err)
		}
	case "otel":
		otelConfig, err := uc.GenerateOtelConfig(hostInfo)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		// Make sure the output directory exists before generating configs.
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("can't create output directory %q: %w", outDir, err)
		}
		path := filepath.Join(outDir, "otel.yaml")
		if err := ioutil.WriteFile(path, []byte(otelConfig), 0644); err != nil {
			return fmt.Errorf("can't write %q: %w", path, err)
		}
	default:
		return fmt.Errorf("unknown service %q", service)
	}
	return nil
}
