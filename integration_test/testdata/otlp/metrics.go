package main

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

func installMetricExportPipeline(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	metricProvider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricsdk.NewPeriodicReader(exporter)),
		metricsdk.WithResource(resource.Default()),
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
	testGaugeMetric(meter, "otlp.test.gauge")

	// Test domain-prefixing
	testGaugeMetric(meter, "workload.googleapis.com/otlp.test.prefix1")
	testGaugeMetric(meter, ".invalid.googleapis.com/otlp.test.prefix2")
	testGaugeMetric(meter, "otlp.test.prefix3/workload.googleapis.com/abc")
	testGaugeMetric(meter, "WORKLOAD.GOOGLEAPIS.COM/otlp.test.prefix4")
	testGaugeMetric(meter, "WORKLOAD.googleapis.com/otlp.test.prefix5")

	// Test cumulative metrics
	counter, err := meter.Float64Counter("otlp.test.cumulative")
	if err != nil {
		log.Fatal(err)
	}
	// Counters need two samples with a short delay in between
	counter.Add(ctx, 5)
	time.Sleep(1 * time.Second)
	counter.Add(ctx, 10)
}

func testGaugeMetric(meter metric.Meter, name string) {
	gauge, err := meter.Float64ObservableGauge(name)
	if err != nil {
		log.Fatal(err)
	}
	_, err = meter.RegisterCallback(func(c context.Context, observer metric.Observer) error {
		observer.ObserveFloat64(gauge, 5)
		return nil
	}, gauge)
	if err != nil {
		log.Fatal(err)
	}
}
