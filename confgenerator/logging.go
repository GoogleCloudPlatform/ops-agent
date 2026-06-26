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

package confgenerator

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

const InstrumentationSourceLabel = `labels."logging.googleapis.com/instrumentation_source"`
const HttpRequestKey = "logging.googleapis.com/httpRequest"

func setLogNameProcessor(ctx context.Context, logName string) LoggingProcessorModifyFields {
	p := platform.FromContext(ctx)
	hostName := p.Hostname()
	fields := map[string]*ModifyField{
		"logName": {
			DefaultValue: &logName,
		},
		`labels."compute.googleapis.com/resource_name"`: {
			DefaultValue: &hostName,
		},
	}
	r, err := p.GetResource()
	if err == nil {
		for k, v := range r.ExtraLogLabels() {
			v := v
			fields[fmt.Sprintf("labels.%q", k)] = &ModifyField{
				DefaultValue: &v,
			}
		}
	}
	return LoggingProcessorModifyFields{
		Fields: fields,
	}
}

func otelSetLogNameComponents(ctx context.Context, logName string) []otel.Component {
	p := setLogNameProcessor(ctx, logName)
	components, err := p.Processors(ctx)
	if err != nil {
		// We're generating a hard-coded config, so this should never fail.
		panic(err)
	}
	return components
}

func otelFluentForwardSetLogNameComponents() []otel.Component {
	bodyFluentTag := ottl.LValue{"body", "fluent.tag"}
	logName := ottl.LValue{"attributes", "gcp.log_name"}

	return []otel.Component{
		otel.Transform(
			"log", "log",
			ottl.NewStatements(
				logName.SetIf(ottl.Concat([]ottl.Value{logName, bodyFluentTag}, "."),
					ottl.And(logName.IsPresent(), bodyFluentTag.IsPresent())),
				bodyFluentTag.DeleteIf(bodyFluentTag.IsPresent()),
			),
		),
	}
}
