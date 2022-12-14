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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	yaml "github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/host"
	"go.uber.org/multierr"
)

var (
	service  = flag.String("service", "", "service to generate config for")
	outDir   = flag.String("out", os.Getenv("RUNTIME_DIRECTORY"), "directory to write configuration files to")
	input    = flag.String("in", "/etc/google-cloud-ops-agent/config.yaml", "path to the user specified agent config")
	logsDir  = flag.String("logs", "/var/log/google-cloud-ops-agent", "path to store agent logs")
	stateDir = flag.String("state", "/var/lib/google-cloud-ops-agent", "path to store agent state like buffers")
	detect   = flag.Bool("detect", false, "Whether to automatically detect integrations and generate config")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("The agent config file is not valid. Detailed error: %s", err)
	}
}
func run() error {
	if *detect {
		return detectAutoConfigs()
	}

	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	builtInConfig, mergedConfig, err := confgenerator.MergeConfFiles(*input, "linux", apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	// Log the built-in and merged config files to STDOUT. These are then written
	// by journald to var/log/syslog and so to Cloud Logging once the ops-agent is
	// running.
	log.Printf("Built-in config:\n%s", builtInConfig)
	log.Printf("Merged config:\n%s", mergedConfig)

	hostInfo, err := host.Info()
	if err != nil {
		return err
	}
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(mergedConfig, hostInfo.OS)
	if err != nil {
		return err
	}
	return confgenerator.GenerateFilesFromConfig(&uc, *service, *logsDir, *stateDir, *outDir)
}

func detectAutoConfigs() error {
	var multiErr error
	uc := confgenerator.UnifiedConfig{
		Logging: &confgenerator.Logging{
			Receivers:  map[string]confgenerator.LoggingReceiver{},
			Processors: map[string]confgenerator.LoggingProcessor{},
			Service: &confgenerator.LoggingService{
				Pipelines: map[string]*confgenerator.Pipeline{},
			},
		},
		Metrics: &confgenerator.Metrics{
			Receivers:  map[string]confgenerator.MetricsReceiver{},
			Processors: map[string]confgenerator.MetricsProcessor{},
			Service: &confgenerator.MetricsService{
				Pipelines: map[string]*confgenerator.Pipeline{},
			},
		},
	}
	for _, app := range []struct {
		app    string
		detect func() ([]confgenerator.LoggingReceiver, []confgenerator.MetricsReceiver, error)
	}{
		{
			app:    "apache",
			detect: apps.ApacheDetectConfigs,
		},
	} {
		logging, metrics, err := app.detect()
		if err != nil {
			multiErr = multierr.Append(multiErr, fmt.Errorf("%s: %v", app.app, err))
			continue
		}
		loggingMap := appendComponents(app.app, logging, uc.Logging.Receivers)
		metricsMap := appendComponents(app.app, metrics, uc.Metrics.Receivers)
		generatePipelines(app.app, loggingMap, uc.Logging.Service.Pipelines)
		generatePipelines(app.app, metricsMap, uc.Metrics.Service.Pipelines)
	}

	if multiErr != nil {
		return multiErr
	}

	yamlBytes, err := yaml.Marshal(uc)
	if err != nil {
		return err
	}
	log.Printf("Detected the following configuration automatically:\n\n%s", string(yamlBytes))
	return nil
}

// appendComponents takes a slice of components, generates names for them, and appends them
// to the given map. A partial map containing only the added elements is returned.
func appendComponents[C any](app string, comps []C, m map[string]C) map[string]C {
	result := map[string]C{}
	for i, comp := range comps {
		name := fmt.Sprintf("%s_%d", app, i+1)
		m[name] = comp
		result[name] = comp
	}
	return result
}

func generatePipelines[C any](app string, m map[string]C, p map[string]*confgenerator.Pipeline) {
	if len(m) > 0 {
		p[app] = &confgenerator.Pipeline{
			ReceiverIDs: make([]string, 0),
		}
		for k := range m {
			p[app].ReceiverIDs = append(p[app].ReceiverIDs, k)
		}
	}
}
