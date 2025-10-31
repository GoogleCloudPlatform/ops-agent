package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

var renameMap = map[string]string{
	"otlp.test.prefix1":       "workload.googleapis.com/otlp.test.prefix1",
	"otlp.test.prefix2":       ".invalid.googleapis.com/otlp.test.prefix2",
	"abc":                     "otlp.test.prefix3/workload.googleapis.com/abc",
	"otlp.test.prefix4":       "WORKLOAD.GOOGLEAPIS.COM/otlp.test.prefix4",
	"otlp.test.prefix5":       "WORKLOAD.googleapis.com/otlp.test.prefix5",
}

var (
	flagServiceName       = flag.String("service_name", "", "service.name attribute value")
	flagServiceNamespace  = flag.String("service_namespace", "", "service.namespace attribute value")
	flagServiceInstanceID = flag.String("service_instance_id", "", "service.instance.id attribute value")
	flagServiceVersion    = flag.String("service_version", "", "service.version attribute value")
)

func getServiceAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	if *flagServiceName != "" {
		attrs = append(attrs, semconv.ServiceNameKey.String(*flagServiceName))
	}
	if *flagServiceNamespace != "" {
		attrs = append(attrs, semconv.ServiceNamespaceKey.String(*flagServiceNamespace))
	}
	if *flagServiceInstanceID != "" {
		attrs = append(attrs, semconv.ServiceInstanceIDKey.String(*flagServiceInstanceID))
	}
	if *flagServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersionKey.String(*flagServiceVersion))
	}
	return attrs
}

func installMetricExportPipeline(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	metricResource := resource.NewWithAttributes("", getServiceAttributes()...)
	metricProvider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricsdk.NewPeriodicReader(exporter)),
		metricsdk.WithResource(metricResource),
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
	flag.Parse()
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

	testHistogramMetric(ctx, meter, "otlp.test.histogram")
	testUpDownCounterMetric(ctx, meter, "otlp.test.updowncounter")
	testCumulativeMetric(ctx, meter, "otlp.test.cumulative")
}

func testCumulativeMetric(ctx context.Context, meter metric.Meter, name string) {
	counter, err := meter.Float64Counter(name)
	if err != nil {
		log.Fatal(err)
	}
	// Counters need two samples with a short delay in between
	counter.Add(ctx, 5)
	time.Sleep(1 * time.Second)
	counter.Add(ctx, 10)
}

func testHistogramMetric(ctx context.Context, meter metric.Meter, name string) {
	histogram, err := meter.Float64Histogram(name)
	if err != nil {
		log.Fatal(err)
	}
	histogram.Record(ctx, 1.0)
	histogram.Record(ctx, 2.0)
	histogram.Record(ctx, 3.0)
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

func testUpDownCounterMetric(ctx context.Context, meter metric.Meter, name string) {
	upDownCounter, err := meter.Int64UpDownCounter(name)
	if err != nil {
		log.Fatal(err)
	}
	// adds and subtracts values from the UpDownCounter
	upDownCounter.Add(ctx, 5)
	time.Sleep(1 * time.Second)
	upDownCounter.Add(ctx, -2)
}
