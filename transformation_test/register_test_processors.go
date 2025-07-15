// Copyright 2025 Google LLC
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

package transformation_test

import (
	"context"

	_ "github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	_ "google.golang.org/grpc/encoding/gzip"
)

// LoggingProcessorTestOtelWindowsEventLog is used to test
type LoggingProcessorTestOtelWindowsEventLog struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (r LoggingProcessorTestOtelWindowsEventLog) Type() string {
	return "windows_event_log_otel_processor"
}

func (p LoggingProcessorTestOtelWindowsEventLog) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	return []fluentbit.Component{}
}

func (p LoggingProcessorTestOtelWindowsEventLog) Processors(ctx context.Context) ([]otel.Component, error) {
	return confgenerator.WindowsEventLogV1Processors(ctx)
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorTestOtelWindowsEventLog{} })
}
