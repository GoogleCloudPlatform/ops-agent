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
	// "flag"
	"fmt"
	"log"
	// "os"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2/google"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

func constructTimeSeriesRequest(ctx context.Context, eR confgenerator.EnabledReceivers) (*monitoringpb.CreateTimeSeriesRequest, error) {
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return nil, err
	}

	projectID := creds.ProjectID
	instance_id, err := metadata.InstanceID()
	if err != nil {
		return nil, err
	}
	zone, err := metadata.Zone()
	if err != nil {
		return nil ,err
	}

	log.Println("projectID : ", projectID)
	log.Println("instance_id : ", instance_id)
	log.Println("zone : ", zone)

	now := &timestamp.Timestamp{
		Seconds: time.Now().Unix(),
	}

	timeSeriesList := []*monitoringpb.TimeSeries{}
	for rType, count := range eR {
		tSeries := monitoringpb.TimeSeries{
			MetricKind: metricpb.MetricDescriptor_GAUGE,
			ValueType:  metricpb.MetricDescriptor_INT64,
			Metric: &metricpb.Metric{
				Type: "agent.googleapis.com/agent/ops_agent/enabled_receivers",
				Labels: map[string]string{
					"receiver_type":  rType,
					"telemetry_type": "metrics",
				},
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "gce_instance",
				Labels: map[string]string{
					"instance_id": instance_id,
					"zone":        zone,
				},
			},
			Points: []*monitoringpb.Point{{
				Interval: &monitoringpb.TimeInterval{
					StartTime: now,
					EndTime:   now,
				},
				Value: &monitoringpb.TypedValue{
					Value: &monitoringpb.TypedValue_Int64Value{
						Int64Value: int64(count),
					},
				},
			}},
		}

		timeSeriesList = append(timeSeriesList, &tSeries)
	}

	req := &monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + projectID,
		TimeSeries: timeSeriesList,
	}

	log.Printf("writeTimeseriesRequest: %+v\n", req)

	return req, nil
}

func sendMetric(eR confgenerator.EnabledReceivers) error {
	fmt.Println("interval", eR)
	ctx := context.Background()

	req, err := constructTimeSeriesRequest(ctx, eR)
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
		log.Printf("could not write time series value, %v ", err)
		return fmt.Errorf("could not write time series value, %v ", err)
	}

	return nil
}

func SendMetricEveryInterval(eR confgenerator.EnabledReceivers, interval int) error {
	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)

		for t := range ticker.C {
			fmt.Println("Tick send metric : %s", t)
			sendMetric(eR)
		}
	}()

	select {}

	return nil
}