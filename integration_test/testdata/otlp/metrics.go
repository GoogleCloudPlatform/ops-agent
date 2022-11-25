package main

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

func installMetricExportPipeline(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	metricProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(resource.Default()),
	)
	global.SetMeterProvider(metricProvider)
	return metricProvider.Shutdown, nil
}

func main() {
	ctx := context.Background()

	shutdown, err := installMetricExportPipeline(ctx)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	meter := global.MeterProvider().Meter("foo")

	// Test gauge metrics
	gauge, err := meter.AsyncFloat64().Gauge("otlp.test.gauge")
	if err != nil {
		log.Fatal(err)
	}
	err = meter.RegisterCallback([]instrument.Asynchronous{gauge}, func(c context.Context) {
		gauge.Observe(c, 5)
	})
	if err != nil {
		log.Fatal(err)
	}

	// Test cumulative metrics
	counter, err := meter.SyncFloat64().Counter("otlp.test.cumulative")
	if err != nil {
		log.Fatal(err)
	}
	// Counters need two samples with a short delay in between
	counter.Add(ctx, 5)
	time.Sleep(1 * time.Second)
	counter.Add(ctx, 10)
}
