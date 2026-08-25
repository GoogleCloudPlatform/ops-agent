package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	serviceName := flag.String("service_name", "my-service", "service.name attribute value")
	serviceNamespace := flag.String("service_namespace", "my-namespace", "service.namespace attribute value")
	serviceInstanceId := flag.String("service_instance_id", "my-instance", "service.instance.id attribute value")
	scopeName := flag.String("scope_name", "my-scope", "scope.name attribute value")
	scopeVersion := flag.String("scope_version", "1.0.0", "scope.version attribute value")
	logBody := flag.String("log_body", "This is a test log", "log body text")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "localhost:4317", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := plogotlp.NewGRPCClient(conn)

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", *serviceName)
	rl.Resource().Attributes().PutStr("service.namespace", *serviceNamespace)
	rl.Resource().Attributes().PutStr("service.instance.id", *serviceInstanceId)

	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName(*scopeName)
	sl.Scope().SetVersion(*scopeVersion)

	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	lr.Body().SetStr(*logBody)

	req := plogotlp.NewExportRequestFromLogs(ld)
	var exportErr error
	for attempt := 1; attempt <= 10; attempt++ {
		exportCtx, exportCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, exportErr = client.Export(exportCtx, req)
		exportCancel()
		if exportErr == nil {
			log.Println("Successfully exported logs")
			return
		}
		log.Printf("client.Export() attempt %d failed: %v, retrying...", attempt, exportErr)
		time.Sleep(1 * time.Second)
	}
	log.Fatalf("could not export after retries: %v", exportErr)
}
