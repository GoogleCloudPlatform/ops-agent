// Copyright 2021 Google LLC
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

// Package confgenerator provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"fmt"
	"os"

	yaml "github.com/goccy/go-yaml"
)

// MergeConfFiles merges the user provided config with the built-in config struct for the platform.
// It returns the built-in config for the platform and the merged config.
func MergeConfFiles(userConfPath, platform string, builtInConfStructs map[string]*UnifiedConfig, detected *UnifiedConfig) ([]byte, []byte, error) {
	mergedConf, err := mergeConfFiles(userConfPath, platform, builtInConfStructs, detected)
	if err != nil {
		return nil, nil, err
	}

	builtInStruct := builtInConfStructs[platform]
	builtInYaml, err := yaml.Marshal(builtInStruct)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert the built-in config for %s to yaml: %w \n", platform, err)
	}

	mergedConfigYaml, err := yaml.Marshal(mergedConf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert the merged config %+v to yaml: %w \n", mergedConf, err)
	}

	return builtInYaml, mergedConfigYaml, nil
}

func mergeConfFiles(userConfPath, platform string, builtInConfStructs map[string]*UnifiedConfig, detected *UnifiedConfig) (*UnifiedConfig, error) {
	builtInStruct := builtInConfStructs[platform]

	// Read the built-in config file.
	original, err := builtInStruct.DeepCopy(platform)
	if err != nil {
		return nil, err
	}

	// Optionally merge the user config file.
	if _, err = os.Stat(userConfPath); err != nil {
		if os.IsNotExist(err) {
			// Skip the merge if the user config file does not exist.
		} else {
			return nil, fmt.Errorf("failed to retrieve the user config file %q: %w \n", userConfPath, err)
		}
	} else {
		overrides, err := ReadUnifiedConfigFromFile(userConfPath, platform)
		if err != nil {
			return nil, err
		}
		mergeConfigs(&original, &overrides)
	}

	return CombineConfigs(&original, detected, "main", "detected", "linux")
}

func mergeConfigs(original, overrides *UnifiedConfig) {
	original.Combined = overrides.Combined
	// For "default_pipeline", we go one level deeper.
	// this covers 2 cases:
	// 1. if "<receivers / processors / exporters>: []" is specified explicitly in user config, the entity gets cleared.
	// 2. if "<receivers / processors / exporters>" is a non-empty list, it overrides the built-in list.
	//    e.g. users might use the config below to turn off iis metrics on windows.
	//    metrics:
	//      service:
	//        pipelines:
	//          default_pipeline:
	//            receivers: [hostmetrics,mssql]
	if overrides.Logging != nil {
		// Overrides logging.receivers.
		for k, v := range overrides.Logging.Receivers {
			original.Logging.Receivers[k] = v
		}

		// Overrides logging.processors.
		original.Logging.Processors = map[string]LoggingProcessor{}
		for k, v := range overrides.Logging.Processors {
			original.Logging.Processors[k] = v
		}
		// Skip deprecated logging.exporters.
		// Override logging.service.pipelines
		if overrides.Logging.Service != nil {
			if overrides.Logging.Service.LogLevel != "info" {
				original.Logging.Service.LogLevel = overrides.Logging.Service.LogLevel
			}
			for name, pipeline := range overrides.Logging.Service.Pipelines {
				// skips logging.service.pipelines.*.exporters
				pipeline.ExporterIDs = nil
				if name == "default_pipeline" {
					// overrides logging.service.pipelines.default_pipeline.receivers
					if ids := pipeline.ReceiverIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ReceiverIDs = ids
					}

					// overrides logging.service.pipelines.default_pipeline.processors
					if ids := pipeline.ProcessorIDs; ids != nil {
						original.Logging.Service.Pipelines["default_pipeline"].ProcessorIDs = ids
					}
				} else {
					// Overrides logging.service.pipelines.<non_default_pipelines>
					original.Logging.Service.Pipelines[name] = pipeline
				}
			}
		}
	}
	if overrides.Metrics != nil {
		// Overrides metrics.receivers.
		for k, v := range overrides.Metrics.Receivers {
			original.Metrics.Receivers[k] = v
		}

		// Overrides metrics.processors.
		for k, v := range overrides.Metrics.Processors {
			original.Metrics.Processors[k] = v
		}

		if overrides.Metrics.Service != nil {
			if overrides.Metrics.Service.LogLevel != "info" {
				original.Metrics.Service.LogLevel = overrides.Metrics.Service.LogLevel
			}
			for name, pipeline := range overrides.Metrics.Service.Pipelines {
				// skips metrics.service.pipelines.*.exporters
				pipeline.ExporterIDs = nil
				// Overrides metrics.service.pipelines.*
				original.Metrics.Service.Pipelines[name] = pipeline
			}
		}
	}
}

// CombineConfigs takes two UnifiedConfigs and combine the receivers, processors
// and service by adding mainSuffix and additionSuffix respectively
func CombineConfigs(main, addition *UnifiedConfig, mainSuffix, additionSuffix, platform string) (*UnifiedConfig, error) {
	combined, err := main.DeepCopy(platform)
	if err != nil {
		return &combined, err
	}
	if addition.Logging != nil {
		// if addition has logging section
		// First append the suffix to the existing components
		if combined.Logging != nil {
			addSuffixForLogging(mainSuffix, &combined)
		}
		// Then append the suffix to the new addition components
		addSuffixForLogging(additionSuffix, addition)
		// Add new components to the final combined config
		for k, v := range addition.Logging.Receivers {
			combined.Logging.Receivers[k] = v
		}
		for k, v := range addition.Logging.Processors {
			combined.Logging.Processors[k] = v
		}
		if addition.Logging.Service != nil {
			for k, v := range addition.Logging.Service.Pipelines {
				combined.Logging.Service.Pipelines[k] = v
			}
		}
	}
	if addition.Metrics != nil {
		if combined.Metrics != nil {
			addSuffixForMetrics(mainSuffix, &combined)
		}
		addSuffixForMetrics(additionSuffix, addition)
		for k, v := range addition.Metrics.Receivers {
			combined.Metrics.Receivers[k] = v
		}
		for k, v := range addition.Metrics.Processors {
			combined.Metrics.Processors[k] = v
		}
		if addition.Metrics.Service != nil {
			for k, v := range addition.Metrics.Service.Pipelines {
				combined.Metrics.Service.Pipelines[k] = v
			}
		}
	}
	return &combined, nil
}

// addSuffixForLogging add suffix to the name of receivers, processors
// and pipelines, and update the receivers & processors name in the pipelines
func addSuffixForLogging(suffix string, uc *UnifiedConfig) {
	uc.Logging.Receivers = addSuffixForComponents(suffix, uc.Logging.Receivers)
	uc.Logging.Processors = addSuffixForComponents(suffix, uc.Logging.Processors)
	addSuffixForPipelines(suffix, uc.Logging.Service.Pipelines)
	uc.Logging.Service.Pipelines = addSuffixForComponents(suffix, uc.Logging.Service.Pipelines)
}

// addSuffixForMetrics add suffix to the name of receivers, processors
// and pipelines, and update the receivers & processors name in the pipelines
func addSuffixForMetrics(suffix string, uc *UnifiedConfig) {
	uc.Metrics.Receivers = addSuffixForComponents(suffix, uc.Metrics.Receivers)
	uc.Metrics.Processors = addSuffixForComponents(suffix, uc.Metrics.Processors)
	addSuffixForPipelines(suffix, uc.Metrics.Service.Pipelines)
	uc.Metrics.Service.Pipelines = addSuffixForComponents(suffix, uc.Metrics.Service.Pipelines)
}

func addSuffixForComponents[C any](suffix string, comps map[string]C) map[string]C {
	modified := map[string]C{}
	for name, comp := range comps {
		newName := addSuffix(name, suffix)
		modified[newName] = comp
	}
	return modified
}

func addSuffixForPipelines(suffix string, ps map[string]*Pipeline) {
	for _, p := range ps {
		for i, rID := range p.ReceiverIDs {
			p.ReceiverIDs[i] = addSuffix(rID, suffix)
		}
		for i, pID := range p.ProcessorIDs {
			p.ProcessorIDs[i] = addSuffix(pID, suffix)
		}
	}
}

func addSuffix(original, suffix string) string {
	return fmt.Sprintf("%s_%s", original, suffix)
}
