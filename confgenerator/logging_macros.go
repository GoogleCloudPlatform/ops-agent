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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

// LoggingReceiverMacro is a logging component that generates other
// Ops Agent receivers and processors as its implementation.
type LoggingReceiverMacro interface {
	Type() string
	// Expand returns a receiver and slice of logging processors that implement this receivers. This is an intermediate step for receivers that can be implemented in a subagent-agnostic way.
	Expand(ctx context.Context) (InternalOTelReceiver, []InternalOTelProcessor)
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

func (cr loggingReceiverMacroAdapter[LRM]) Expand(ctx context.Context) (InternalOTelReceiver, []InternalOTelProcessor) {
	return cr.ReceiverMacro.Expand(ctx)
}

func (cr loggingReceiverMacroAdapter[LRM]) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	receiver, processors := cr.Expand(ctx)
	rps, err := receiver.Pipelines(ctx)
	if err != nil {
		return nil, err
	}
	for _, pipeline := range rps {
		for _, p := range processors {
			c, err := p.Processors(ctx)
			if err != nil {
				return nil, err
			}
			pipeline.Processors["logs"] = append(pipeline.Processors["logs"], c...)
		}
	}
	return rps, nil
}

// LoggingProcessorMacro is a logging component that generates other
// Ops Agent processors as its implementation.
type LoggingProcessorMacro interface {
	Type() string
	// Expand returns a slice of logging processors that implement this processor. This is an intermediate step for processors that can be implemented in a subagent-agnostic way.
	Expand(ctx context.Context) []InternalOTelProcessor
}

func RegisterLoggingProcessorMacro[LPM LoggingProcessorMacro](platforms ...platform.Type) {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor {
		return &loggingProcessorMacroAdapter[LPM]{}
	}, platforms...)
}

func ReplaceLoggingProcessorMacro[LPM LoggingProcessorMacro](platforms ...platform.Type) {
	LoggingProcessorTypes.ReplaceType(func() LoggingProcessor {
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

func (cp loggingProcessorMacroAdapter[LPM]) Expand(ctx context.Context) []InternalOTelProcessor {
	return cp.ProcessorMacro.Expand(ctx)
}

func (cp loggingProcessorMacroAdapter[LPM]) Processors(ctx context.Context) ([]otel.Component, error) {
	var processors []otel.Component
	for _, lp := range cp.Expand(ctx) {
		c, err := lp.Processors(ctx)
		if err != nil {
			return nil, err
		}
		processors = append(processors, c...)
	}
	return processors, nil
}

// RegisterLoggingFilesProcessorMacro registers a LoggingProcessorMacro as a processor type and also registers a receiver that combines a LoggingReceiverFilesMixin with that LoggingProcessorMacro.
func RegisterLoggingFilesProcessorMacro[LPM LoggingProcessorMacro](filesMixinConstructor func() LoggingReceiverFilesMixin, platforms ...platform.Type) {
	RegisterLoggingProcessorMacro[LPM](platforms...)
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

func (fpma loggingFilesProcessorMacroAdapter[LPM]) Expand(ctx context.Context) (InternalOTelReceiver, []InternalOTelProcessor) {
	return &fpma.LoggingReceiverFilesMixin, fpma.ProcessorMacro.Expand(ctx)
}
