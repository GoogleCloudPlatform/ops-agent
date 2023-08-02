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
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

// getPlatformKey uses the platform to generate the key that can be used to
// obtain the correct built-in configs
func getPlatformKey(platform platform.Platform) string {
	if platform.HasNvidiaGpu {
		return fmt.Sprintf("%s_gpu", platform.Name())
	}
	return platform.Name()
}

// MergeConfFiles merges the user provided config with the built-in config struct for the platform.
func MergeConfFiles(ctx context.Context, userConfPath string, builtInConfStructs map[string]*UnifiedConfig) (*UnifiedConfig, error) {
	builtInStruct := builtInConfStructs[getPlatformKey(platform.FromContext(ctx))]

	// Start with the built-in config.
	result, err := builtInStruct.DeepCopy(ctx)
	if err != nil {
		return nil, err
	}

	overrides, err := ReadUnifiedConfigFromFile(ctx, userConfPath)
	if err != nil {
		return nil, err
	}

	// Optionally merge the user config file.
	if overrides != nil {
		mergeConfigs(result, overrides)
	}

	if err := result.Validate(ctx); err != nil {
		return nil, err
	}

	// Ensure the merged config struct fields are valid.
	v := newValidator()
	if err := v.Struct(result); err != nil {
		panic(err)
	}
	return result, nil
}

func mergeConfigs(original, overrides *UnifiedConfig) {
	// built-in configs do not contain these sections.
	original.Combined = overrides.Combined
	original.Traces = overrides.Traces
	original.Global = overrides.Global

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
