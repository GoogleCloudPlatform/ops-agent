package main

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

var renameMap = map[string]string{
	"otlp.test.prefix1": "workload.googleapis.com/otlp.test.prefix1",
	"otlp.test.prefix2": ".invalid.googleapis.com/otlp.test.prefix2",
	"abc":               "otlp.test.prefix3/workload.googleapis.com/abc",
	"otlp.test.prefix4": "WORKLOAD.GOOGLEAPIS.COM/otlp.test.prefix4",
	"otlp.test.prefix5": "WORKLOAD.googleapis.com/otlp.test.prefix5",
}

func installMetricExportPipeline(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	metricProvider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricsdk.NewPeriodicReader(exporter)),
		metricsdk.WithResource(resource.Default()),
		metricsdk.WithView(func(i metricsdk.Instrument) (metricsdk.Stream, bool) {
			s := metricsdk.Stream{Name: i.Name, Description: i.Description, Unit: i.Unit}
			newName, ok := renameMap[i.Name]
			if !ok {
				return s, false
			}
			s.Name = newName
			return s, true
		}),
	)
	otel.SetMeterProvider(metricProvider)
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

	meter := otel.GetMeterProvider().Meter("foo")

	// Test gauge metrics
	testGaugeMetric(meter, "otlp.test.gauge")

	// Test domain-prefixing, since these metrics have a domain added by a view
	testGaugeMetric(meter, "otlp.test.prefix1")
	testGaugeMetric(meter, "otlp.test.prefix2")
	testGaugeMetric(meter, "abc")
	testGaugeMetric(meter, "otlp.test.prefix4")
	testGaugeMetric(meter, "otlp.test.prefix5")

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
