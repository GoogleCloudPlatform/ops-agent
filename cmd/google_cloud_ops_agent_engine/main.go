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
	"log"
	"os"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/healthchecks"
)

var (
	service  = flag.String("service", "", "service to generate config for")
	outDir   = flag.String("out", os.Getenv("RUNTIME_DIRECTORY"), "directory to write configuration files to")
	input    = flag.String("in", "/etc/google-cloud-ops-agent/config.yaml", "path to the user specified agent config")
	logsDir  = flag.String("logs", "/var/log/google-cloud-ops-agent", "path to store agent logs")
	stateDir = flag.String("state", "/var/lib/google-cloud-ops-agent", "path to store agent state like buffers")
)

func runStartupChecks(service string) error {
	// To run checks in each subagent service we could
	// use a switch to define the checks as follows.
	/* switch service {
		case "":
		case "fluentbit":
		case "otel":
	} */
	var GCEHealthChecks healthchecks.HealthCheckRegistry
	switch service {
	case "":
		GCEHealthChecks = healthchecks.HealthCheckRegistry{
			healthchecks.PortsCheck{},
			healthchecks.NetworkCheck{},
			healthchecks.APICheck{},
		}
		// case "fluentbit":
		// case "otel":
	}

	healthCheckResults, err := GCEHealthChecks.RunAllHealthChecks(*logsDir)
	if err != nil {
		return err
	}
	for _, message := range healthCheckResults {
		log.Printf(message)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("The agent config file is not valid. Detailed error: %s", err)
	}
}
func run() error {
	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	uc, err := confgenerator.MergeConfFiles(*input, "linux", apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	// Log the built-in and merged config files to STDOUT. These are then written
	// by journald to var/log/syslog and so to Cloud Logging once the ops-agent is
	// running.
	log.Printf("Built-in config:\n%s", apps.BuiltInConfStructs["linux"])
	log.Printf("Merged config:\n%s", uc)

	if err := runStartupChecks(*service); err != nil {
		return err
	}

	return confgenerator.GenerateFilesFromConfig(uc, *service, *logsDir, *stateDir, *outDir)
}
