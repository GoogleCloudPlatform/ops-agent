// Copyright 2022 Google LLC
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
	"os/exec"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"gopkg.in/natefinch/lumberjack.v2"
)

func getMergedConfigForPlatform(userConfPath, platform string) (*confgenerator.UnifiedConfig, error) {
	mergedUc, err := confgenerator.MergeConfFiles(userConfPath, platform, apps.BuiltInConfStructs)
	if err != nil {
		return nil, err
	}

	return mergedUc, nil
}

func getLogFileRotation(config *confgenerator.UnifiedConfig) confgenerator.LogFileRotation {
	if config.Global == nil || config.Global.DefaultLogFileRotation == nil {
		return confgenerator.LogFileRotation{}
	}
	return *config.Global.DefaultLogFileRotation
}

func run(logFilename, configurationPath string, command []string) (*exec.Cmd, error) {
	ucConfig, err := getMergedConfig(configurationPath)
	if err != nil {
		return nil, err
	}
	config := getLogFileRotation(ucConfig)

	cmd := exec.Command(command[0], command[1:]...)
	if logFilename != "" && config.GetEnabled() {
		logger := lumberjack.Logger{
			Filename:   logFilename,
			MaxSize:    config.GetMaxFileSize(),
			MaxBackups: config.GetBackupCount(),
			Compress:   false,
		}
		defer logger.Close()
		_, err = logger.Write([]byte{}) // Empty write to ensure file can be opened
		if err != nil {
			return nil, err
		}
		cmd.Stdout = &logger
	} else if logFilename != "" {
		file, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		cmd.Stdout = file
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = cmd.Stdout
	if err := runCommand(cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

var logPathFlag = flag.String("log_path", "", "The name of the file to log to. If empty, logs to stdout")
var configurationPathFlag = flag.String("config_path", "", "The path to the user specified agent config")

func main() {
	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
		log.Fatal("Command to run must be passed in as first argument")
	}
	cmd, err := run(*logPathFlag, *configurationPathFlag, flag.Args())
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(cmd.ProcessState.ExitCode())
}
