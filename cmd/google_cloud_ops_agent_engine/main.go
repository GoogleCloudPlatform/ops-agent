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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

var (
	service  = flag.String("service", "", "service to generate config for")
	outDir   = flag.String("out", os.Getenv("RUNTIME_DIRECTORY"), "directory to write configuration files to")
	input    = flag.String("in", "/etc/google-cloud-ops-agent/config.yaml", "path to read the user specified agent config")
	builtin  = flag.String("builtin", "/etc/google-cloud-ops-agent/debugging/built-in-config.yaml", "path to write the built-in agent config for debugging purpose")
	merged   = flag.String("merged", "/etc/google-cloud-ops-agent/debugging/merged-config.yaml", "path to write the merged agent config for debugging purpose")
	logsDir  = flag.String("logs", "/var/log/google-cloud-ops-agent", "path to store agent logs")
	stateDir = flag.String("state", "/var/lib/google-cloud-ops-agent", "path to store agent state like buffers")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if err := confgenerator.MergeConfFiles(*builtin, *input, *merged, "linux"); err != nil {
		return err
	}
	return confgenerator.GenerateFiles(*merged, *service, *logsDir, *stateDir, *outDir)
}
