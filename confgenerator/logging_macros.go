// Copyright 2025 Google LLC
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
	"errors"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

// LoggingProcessorMacro is a logging processor that generates other Ops Agent processors as its implementation.
type LoggingProcessorMacro interface {
	Type() string
	// Processors returns slice of logging processors. This is an intermediate representation before sub-agent specific configurations.
	InternalLoggingProcessors(ctx context.Context) []InternalLoggingProcessor
}

func RegisterLoggingProcessorMacro[LPM LoggingProcessorMacro]() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor {
		return &loggingProcessorMacroAdapter[LPM]{}
	})
}

// loggingProcessorMacroAdapter is the type used to unmarshal user configuration for a LoggingProcessorMacro and adapt its interface to the LoggingProcessor interface.
type loggingProcessorMacroAdapter[P LoggingProcessorMacro] struct {
	ConfigComponent `yaml:",inline"`
	ProcessorMacro  P `yaml:",inline"`
}

func (cp loggingProcessorMacroAdapter[P]) Type() string {
	return cp.ProcessorMacro.Type()
}

func (cp loggingProcessorMacroAdapter[P]) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	var c []fluentbit.Component
	for _, p := range cp.ProcessorMacro.InternalLoggingProcessors(ctx) {
		c = append(c, p.Components(ctx, tag, uid)...)
	}
	return c
}

// LoggingCompositeReceiver represents a pipeline that consists of one log receiver & one or more log processors.
type LoggingCompositeReceiver[R InternalLoggingReceiver, P LoggingProcessorMacro] struct {
	ConfigComponent  `yaml:",inline"`
	ProcessorMacro   P `yaml:",inline"`
	InternalReceiver R `yaml:",inline"`
}

func (cr *LoggingCompositeReceiver[R, P]) Type() string {
	return cr.ProcessorMacro.Type()
}

func (cr *LoggingCompositeReceiver[R, P]) processor() InternalLoggingProcessor {
	return &loggingProcessorMacroAdapter[P]{ProcessorMacro: cr.ProcessorMacro}
}

func (cr *LoggingCompositeReceiver[R, P]) Components(ctx context.Context, tag string) []fluentbit.Component {
	c := cr.InternalReceiver.Components(ctx, tag)
	c = append(c, cr.processor().Components(ctx, tag, fmt.Sprintf("%s", cr.Type()))...)
	return c
}

func (cr *LoggingCompositeReceiver[R, P]) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	if r, ok := any(cr.InternalReceiver).(OTelReceiver); ok {
		rps, err := r.Pipelines(ctx)
		if err != nil {
			return nil, err
		}
		for _, pipeline := range rps {
			if p, ok := any(cr.processor()).(OTelProcessor); ok {
				c, err := p.Processors(ctx)
				if err != nil {
					return nil, err
				}
				pipeline.Processors["logs"] = append(pipeline.Processors["logs"], c...)
			} else {
				return nil, errors.New("unimplemented")
			}
		}
		return rps, nil
	}
	return nil, errors.New("unimplemented")
}
