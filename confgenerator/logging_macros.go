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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

// LoggingReceiverMacro is a logging component that generates other
// Ops Agent receivers and processors as its implementation.
type LoggingReceiverMacro interface {
	Type() string
	// Expand returns a receiver and slice of logging processors that implement this receivers. This is an intermediate step for receivers that can be implemented in a subagent-agnostic way.
	Expand(ctx context.Context) (InternalLoggingReceiver, []InternalLoggingProcessor)
}

func RegisterLoggingReceiverMacro[LRM LoggingReceiverMacro](constructor func() LRM, platforms ...platform.Type) {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver {
		return &loggingReceiverMacroAdapter[LRM]{ReceiverMacro: constructor()}
	}, platforms...)
}

// loggingReceiverMacroAdapter is the type used to unmarshal user configuration for a LoggingReceiverMacro and adapt its interface to the LoggingReceiver interface.
type loggingReceiverMacroAdapter[LRM LoggingReceiverMacro] struct {
	ConfigComponent `yaml:",inline"`
	ReceiverMacro   LRM `yaml:",inline"`
}

func (cr loggingReceiverMacroAdapter[LRM]) Type() string {
	return cr.ReceiverMacro.Type()
}

func (cr loggingReceiverMacroAdapter[LRM]) Expand(ctx context.Context) (InternalLoggingReceiver, []InternalLoggingProcessor) {
	return cr.ReceiverMacro.Expand(ctx)
}

func (cr loggingReceiverMacroAdapter[LRM]) Components(ctx context.Context, tag string) []fluentbit.Component {
	receiver, processors := cr.Expand(ctx)

	c := receiver.Components(ctx, tag)
	for _, p := range processors {
		c = append(c, p.Components(ctx, tag, cr.Type())...)
	}
	return c
}

func (cr loggingReceiverMacroAdapter[LRM]) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	receiver, processors := cr.Expand(ctx)
	if r, ok := any(receiver).(InternalOTelReceiver); ok {
		rps, err := r.Pipelines(ctx)
		if err != nil {
			return nil, err
		}
		for _, pipeline := range rps {
			for _, p := range processors {
				if p, ok := p.(InternalOTelProcessor); ok {
					c, err := p.Processors(ctx)
					if err != nil {
						return nil, err
					}
					pipeline.Processors["logs"] = append(pipeline.Processors["logs"], c...)
				} else {
					return nil, errors.New("unimplemented")
				}
			}
		}
		return rps, nil
	}
	return nil, errors.New("unimplemented")
}

// LoggingProcessorMacro is a logging component that generates other
// Ops Agent processors as its implementation.
type LoggingProcessorMacro interface {
	Type() string
	// Expand returns a slice of logging processors that implement this processor. This is an intermediate step for processors that can be implemented in a subagent-agnostic way.
	Expand(ctx context.Context) []InternalLoggingProcessor
}

func RegisterLoggingProcessorMacro[LPM LoggingProcessorMacro](platforms ...platform.Type) {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor {
		return &loggingProcessorMacroAdapter[LPM]{}
	}, platforms...)
}

// loggingProcessorMacroAdapter is the type used to unmarshal user configuration for a LoggingProcessorMacro and adapt its interface to the LoggingProcessor interface.
type loggingProcessorMacroAdapter[LPM LoggingProcessorMacro] struct {
	ConfigComponent `yaml:",inline"`
	ProcessorMacro  LPM `yaml:",inline"`
}

func (cp loggingProcessorMacroAdapter[LPM]) Type() string {
	return cp.ProcessorMacro.Type()
}

func (cp loggingProcessorMacroAdapter[LPM]) Expand(ctx context.Context) []InternalLoggingProcessor {
	return cp.ProcessorMacro.Expand(ctx)
}

func (cp loggingProcessorMacroAdapter[LPM]) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	var c []fluentbit.Component
	for _, p := range cp.Expand(ctx) {
		c = append(c, p.Components(ctx, tag, uid)...)
	}
	return c
}

func (cp loggingProcessorMacroAdapter[LPM]) Processors(ctx context.Context) ([]otel.Component, error) {
	var processors []otel.Component
	for _, lp := range cp.Expand(ctx) {
		if p, ok := any(lp).(InternalOTelProcessor); ok {
			c, err := p.Processors(ctx)
			if err != nil {
				return nil, err
			}
			processors = append(processors, c...)
		} else {
			return nil, errors.New("unimplemented")
		}
	}
	return processors, nil
}

// RegisterLoggingFilesProcessorMacro registers a LoggingProcessorMacro as a processor type and also registers a receiver that combines a LoggingReceiverFilesMixin with that LoggingProcessorMacro.
func RegisterLoggingFilesProcessorMacro[LPM LoggingProcessorMacro](filesMixinConstructor func() LoggingReceiverFilesMixin, platforms ...platform.Type) {
	RegisterLoggingProcessorMacro[LPM]()
	RegisterLoggingReceiverMacro[*loggingFilesProcessorMacroAdapter[LPM]](func() *loggingFilesProcessorMacroAdapter[LPM] {
		return &loggingFilesProcessorMacroAdapter[LPM]{
			LoggingReceiverFilesMixin: filesMixinConstructor(),
		}
	}, platforms...)
}

type loggingFilesProcessorMacroAdapter[LPM LoggingProcessorMacro] struct {
	LoggingReceiverFilesMixin `yaml:",inline"`
	ProcessorMacro            LPM `yaml:",inline"`
}

func (fpma loggingFilesProcessorMacroAdapter[LPM]) Type() string {
	return fpma.ProcessorMacro.Type()
}

func (fpma loggingFilesProcessorMacroAdapter[LPM]) Expand(ctx context.Context) (InternalLoggingReceiver, []InternalLoggingProcessor) {
	return &fpma.LoggingReceiverFilesMixin, fpma.ProcessorMacro.Expand(ctx)
}
