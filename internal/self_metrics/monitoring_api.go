// Copyright 2020, Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package self_metrics

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	gce_metadata "cloud.google.com/go/compute/metadata"
	oauth2 "golang.org/x/oauth2/google"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

type IntervalMetrics struct {
	Metrics []Metric
	Interval int
}


func constructTimeSeriesRequest(ctx context.Context, metrics []Metric) (*monitoringpb.CreateTimeSeriesRequest, error) {
	creds, err := oauth2.FindDefaultCredentials(ctx)
	if err != nil {
		return nil, err
	}

	projectID := creds.ProjectID
	instance_id, err := gce_metadata.InstanceID()
	if err != nil {
		return nil, err
	}
	zone, err := gce_metadata.Zone()
	if err != nil {
		return nil, err
	}

	log.Println("projectID : ", projectID)
	log.Println("instance_id : ", instance_id)
	log.Println("zone : ", zone)

	timeSeriesList := make([]*monitoringpb.TimeSeries, 0)
	for _, m := range metrics {
		timeSeriesList = append(timeSeriesList, m.ToTimeSeries(instance_id, zone))
	}

	req := &monitoringpb.CreateTimeSeriesRequest{
		Name:       "projects/" + projectID,
		TimeSeries: timeSeriesList,
	}

	log.Printf("writeTimeseriesRequest: %+v\n", req)

	return req, nil
}

func SendMetricsRequest(metrics []Metric) error {
	ctx := context.Background()

	req, err := constructTimeSeriesRequest(ctx, metrics)
	if err != nil {
		return err
	}

	c, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	err = c.CreateTimeSeries(ctx, req)
	if err != nil {
		return fmt.Errorf("could not write time series value, %v ", err)
	}

	return nil
}

func SendMetricsEveryInterval(metrics []IntervalMetrics) error {
	bufferChannel := make(chan []Metric)
    buffer := make([]Metric, 0)

    death := make(chan os.Signal, 1)
    signal.Notify(death, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)

    tickers := make([]*time.Ticker, 0)
    
    for _, m := range metrics {
        tickers = append(tickers, time.NewTicker(time.Duration(m.Interval) * time.Minute))
    }

    for idx, m := range metrics {
        go registerMetric(m, bufferChannel, tickers[idx])
    }

    for {
        select {
        case d := <-bufferChannel:
            if len(buffer) == 0 {
                go waitForBufferChannel(&buffer)
            }
            buffer = append(buffer, d...)

        case <-death:
        	for _, t := range tickers {
        		t.Stop()
        	}
            return nil
        }
    }
}


func registerMetric(metric IntervalMetrics, bufferChannel chan []Metric, ticker *time.Ticker) error {
    for {
        select {
        case <-ticker.C:
            bufferChannel <- metric.Metrics
        }
    }

	return nil
}

func waitForBufferChannel(buffer *[]Metric) {
	// Wait for full buffer
    time.Sleep(time.Duration(30) * time.Second)

    SendMetricsRequest(*buffer)

    // Clear buffer
    *buffer = make([]Metric, 0)
}