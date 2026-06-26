package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
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

func main() {
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

	ctx, span := otel.Tracer("test_tracer", oteltrace.WithInstrumentationVersion("v1.2.3")).Start(ctx, "test_trace")
	span.SetAttributes(attribute.String("custom.span.attribute", "my-span-value"))
	span.End()
}
