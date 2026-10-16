package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	flagDuration = flag.Duration("duration", 20*time.Second, "duration to periodically emit trace spans before exiting")
	flagInterval = flag.Duration("interval", 3*time.Second, "interval between trace span emissions")
)

func installTraceExportPipeline(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	r, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("custom.resource.attribute", "my-resource-value"),
		),
	)
	if err != nil {
		return nil, err
	}
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(r),
	)
	otel.SetTracerProvider(tracerProvider)
	return tracerProvider.Shutdown, nil
}

func emitSpan(ctx context.Context) {
	_, span := otel.Tracer("test_tracer", oteltrace.WithInstrumentationVersion("v1.2.3")).Start(ctx, "test_trace")
	span.SetAttributes(attribute.String("custom.span.attribute", "my-span-value"))
	span.End()
}

func main() {
	flag.Parse()
	ctx := context.Background()

	shutdown, err := installTraceExportPipeline(ctx)
	if err != nil {
		log.Fatalf("Could not install trace pipeline: %v", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Fatalf("Could not shutdown trace pipeline: %v", err)
		}
	}()

	emitSpan(ctx)

	if *flagDuration > 0 && *flagInterval > 0 {
		ticker := time.NewTicker(*flagInterval)
		defer ticker.Stop()
		timeout := time.After(*flagDuration)
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				emitSpan(ctx)
			}
		}
	}
}
