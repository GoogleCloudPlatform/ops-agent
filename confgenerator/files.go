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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/shirou/gopsutil/host"
)

func GenerateFiles(input, service, logsDir, stateDir, outDir string) error {
	hostInfo, _ := host.Info()
	data, err := ioutil.ReadFile(input)
	if err != nil {
		return err
	}
	uc, err := ParseUnifiedConfigAndValidate(data, hostInfo.OS)
	if err != nil {
		return err
	}
	return GenerateFilesFromConfig(&uc, service, logsDir, stateDir, outDir)
}

func ReadUnifiedConfigFromFile(path, platform string) (UnifiedConfig, error) {
	uc := UnifiedConfig{}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return uc, err
	}
	uc, err = UnmarshalYamlToUnifiedConfig(data, platform)
	if err != nil {
		return uc, err
	}
	return uc, nil
}

func GenerateFilesFromConfig(uc *UnifiedConfig, service, logsDir, stateDir, outDir string) error {
	hostInfo, _ := host.Info()
	switch service {
	case "": // Validate-only.
		return nil
	case "fluentbit":
		mainConfig, parserConfig, err := uc.GenerateFluentBitConfigs(logsDir, stateDir, hostInfo)
		if err != nil {
			return fmt.Errorf("can't parse configuration: %w", err)
		}
		if err = writeConfigFile([]byte(mainConfig), filepath.Join(outDir, "fluent_bit_main.conf")); err != nil {
			return err
		}
		if err = writeConfigFile([]byte(parserConfig), filepath.Join(outDir, "fluent_bit_parser.conf")); err != nil {
			return err
		}
		if err = writeForwardScript(filepath.Join(outDir, "add_log_name.lua")); err != nil {
			return err
		}
	case "otel":
		otelConfig, err := uc.GenerateOtelConfig(hostInfo)
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
