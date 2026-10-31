package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

func GenerateFilesFromConfig(ctx context.Context, uc *confgenerator.UnifiedConfig, service, logsDir, stateDir, outDir string) error {
	switch service {
	case "": // Validate-only.
		return nil
	case "fluentbit":
		files, err := uc.GenerateFluentBitConfigs(ctx, logsDir, stateDir)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		for name, contents := range files {
			if err = WriteConfigFile([]byte(contents), filepath.Join(outDir, name)); err != nil {
				return err
			}
		}
	case "otel":
		otelConfig, err := uc.GenerateOtelConfig(ctx, outDir, stateDir, logsDir)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		if err = WriteConfigFile([]byte(otelConfig), filepath.Join(outDir, "otel.yaml")); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown service %q", service)
	}
	return nil
}

func WriteConfigFile(content []byte, path string) error {
	// Make sure the directory exists before writing the file.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %q: %w", path, err)
	}
	content = append(content, []byte("\n")...)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write file to %q: %w", path, err)
	}
	return nil
}

func BuildUnifiedConfigFromFile(ctx context.Context, userConfPath string, defaults ...confgenerator.AgentDefaults) (*confgenerator.UnifiedConfig, error) {
	var data []byte
	if _, err := os.Stat(userConfPath); err == nil {
		d, err := os.ReadFile(userConfPath)
		if err != nil {
			return nil, err
		}
		data = d
	}
	return confgenerator.BuildUnifiedConfig(ctx, data)
}

func ReadUnifiedConfigFromFile(ctx context.Context, path string) (*confgenerator.UnifiedConfig, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve the user config file %q: %w \n", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	uc, err := confgenerator.UnmarshalYamlToUnifiedConfig(ctx, data)
	if err != nil {
		return nil, err
	}

	return uc, nil
}
