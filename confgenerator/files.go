// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package confgenerator

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
)

// ReadUnifiedConfigFromFile reads the user config file and returns a UnifiedConfig.
// If the user config file does not exist, it returns nil.
func ReadUnifiedConfigFromFile(ctx context.Context, path string) (*UnifiedConfig, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// If the user config file does not exist, we don't want any overrides.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve the user config file %q: %w \n", path, err)
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	uc, err := UnmarshalYamlToUnifiedConfig(ctx, data)
	if err != nil {
		return nil, err
	}

	return uc, nil
}

func (uc *UnifiedConfig) GenerateFilesFromConfig(ctx context.Context, service, logsDir, stateDir, outDir string, resource resourcedetector.Resource) error {
	switch service {
	case "": // Validate-only.
		return nil
	case "fluentbit":
		files, err := uc.GenerateFluentBitConfigs(ctx, logsDir, stateDir)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		for name, contents := range files {
			if err = writeConfigFile([]byte(contents), filepath.Join(outDir, name)); err != nil {
				return err
			}
		}
	case "otel":
		// Fetch resource information from the metadata server.
		var err error
		otelConfig, err := uc.GenerateOtelConfig(ctx, resource)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		if err = writeConfigFile([]byte(otelConfig), filepath.Join(outDir, "otel.yaml")); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown service %q", service)
	}
	return nil
}

func writeConfigFile(content []byte, path string) error {
	// Make sure the directory exists before writing the file.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %q: %w", path, err)
	}
	if err := ioutil.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write file to %q: %w", path, err)
	}
	return nil
}
