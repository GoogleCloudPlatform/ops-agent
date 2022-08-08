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

package monitoring_api

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	"os/signal"

	gce_metadata "cloud.google.com/go/compute/metadata"
	oauth2 "golang.org/x/oauth2/google"

	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	monitoring "cloud.google.com/go/monitoring/apiv3"
	time_series "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

func constructTimeSeriesRequest(ctx context.Context, metrics []time_series.Metric) (*monitoringpb.CreateTimeSeriesRequest, error) {
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
		return nil ,err
	}

	log.Println("projectID : ", projectID)
	log.Println("instance_id : ", instance_id)
	log.Println("zone : ", zone)

	timeSeriesList := make([]*monitoringpb.TimeSeries, 0)
	for _, m := range metrics {
		timeSeriesList =  append(timeSeriesList, m.ToTimeSeries(instance_id, zone))
	}

	req := &monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + projectID,
		TimeSeries: timeSeriesList,
	}

	log.Printf("writeTimeseriesRequest: %+v\n", req)

	return req, nil
}

func sendMetric(metrics []time_series.Metric) error {
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

func SendMetricEveryInterval(metrics []time_series.Metric, interval int) error {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)

	death := make(chan os.Signal, 1)
	signal.Notify(death, os.Interrupt, os.Kill)

	for {
		select {
		case <-ticker.C:
			log.Println("Tick send metric : ")
			sendMetric(metrics)
		case <-death:
			ticker.Stop()
			return nil
		}
	}

	return nil
}