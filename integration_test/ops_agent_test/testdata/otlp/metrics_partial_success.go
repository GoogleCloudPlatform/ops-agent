package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	flagServiceName       = flag.String("service_name", "", "service.name attribute value")
	flagServiceNamespace  = flag.String("service_namespace", "", "service.namespace attribute value")
	flagServiceInstanceID = flag.String("service_instance_id", "", "service.instance.id attribute value")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "localhost:4317", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := pmetricotlp.NewGRPCClient(conn)

	md := pmetric.NewMetrics()
	rl := md.ResourceMetrics().AppendEmpty()
	if *flagServiceName != "" {
		rl.Resource().Attributes().PutStr("service.name", *flagServiceName)
	}
	if *flagServiceNamespace != "" {
		rl.Resource().Attributes().PutStr("service.namespace", *flagServiceNamespace)
	}
	if *flagServiceInstanceID != "" {
		rl.Resource().Attributes().PutStr("service.instance.id", *flagServiceInstanceID)
	}

	sl := rl.ScopeMetrics().AppendEmpty()
	sl.Scope().SetName("foo")

	// 1. Good metric
	m1 := sl.Metrics().AppendEmpty()
	m1.SetName("otlp.test.gauge_good")
	m1.SetEmptyGauge()
	dp1 := m1.Gauge().DataPoints().AppendEmpty()
	ts := pcommon.NewTimestampFromTime(time.Now())
	dp1.SetTimestamp(ts)
	dp1.SetDoubleValue(33)

	// 2. Metric with duplicate points (triggers partial success)
	m2 := sl.Metrics().AppendEmpty()
	m2.SetName("otlp.test.gauge_duplicate")
	m2.SetEmptyGauge()
	
	dp2 := m2.Gauge().DataPoints().AppendEmpty()
	dp2.SetTimestamp(ts)
	dp2.SetDoubleValue(5)

	dp3 := m2.Gauge().DataPoints().AppendEmpty()
	dp3.SetTimestamp(ts) // Same timestamp!
	dp3.SetDoubleValue(10)

	req := pmetricotlp.NewExportRequestFromMetrics(md)
	_, err = client.Export(ctx, req)
	if err != nil {
		log.Fatalf("could not export: %v", err)
	}
	log.Println("Successfully exported metrics")
}
