// Copyright 2020, Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"log"

	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"go.uber.org/zap"
)

type FileLogger struct {
	logger *zap.SugaredLogger
}

func New(file string) *FileLogger {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{
		file,
	}
	logger, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		log.Fatal(err)
	}

	sugar := logger.Sugar().With(
		zap.String("ops-agent-version", version.Version))
	return &FileLogger{
		logger: sugar,
	}
}

func DiscardLogger() *FileLogger {
	return New("/dev/null")
}

func Default() *FileLogger {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	sugar := logger.Sugar().With(
		zap.String("version", version.Version))
	return &FileLogger{
		logger: sugar,
	}
}

func (f FileLogger) Printf(format string, v ...any) {
	f.logger.Infof(format, v...)
}

func (f FileLogger) Println(v ...any) {
	f.logger.Infoln(v...)
}
