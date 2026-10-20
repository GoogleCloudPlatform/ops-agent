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

var (
	serviceName       = flag.String("service_name", "my-service", "service.name attribute value")
	serviceNamespace  = flag.String("service_namespace", "my-namespace", "service.namespace attribute value")
	serviceInstanceId = flag.String("service_instance_id", "my-instance", "service.instance.id attribute value")
	scopeName         = flag.String("scope_name", "my-scope", "scope.name attribute value")
	scopeVersion      = flag.String("scope_version", "1.0.0", "scope.version attribute value")
	logBody           = flag.String("log_body", "This is a test log", "log body text")
	flagDuration      = flag.Duration("duration", 20*time.Second, "duration to periodically emit logs before exiting")
	flagInterval      = flag.Duration("interval", 3*time.Second, "interval between log emissions")
)

func emitLog(ctx context.Context, client plogotlp.GRPCClient) error {
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
	_, err := client.Export(ctx, req)
	return err
}

func main() {
	flag.Parse()

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "localhost:4317", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := plogotlp.NewGRPCClient(conn)

	var exportSucceeded bool
	if err := emitLog(ctx, client); err != nil {
		log.Printf("initial export failed: %v", err)
	} else {
		exportSucceeded = true
		log.Println("Successfully exported logs")
	}

	if *flagDuration > 0 && *flagInterval > 0 {
		ticker := time.NewTicker(*flagInterval)
		defer ticker.Stop()
		timeout := time.After(*flagDuration)
		for {
			select {
			case <-timeout:
				if !exportSucceeded {
					log.Fatalf("failed to export logs within duration %v", *flagDuration)
				}
				return
			case <-ticker.C:
				if err := emitLog(ctx, client); err != nil {
					log.Printf("could not export: %v", err)
				} else {
					exportSucceeded = true
				}
			}
		}
	} else if !exportSucceeded {
		log.Fatalf("could not export logs")
	}
}
