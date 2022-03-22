//go:build integration_test

/*
PROJECT: The GCP project to use.
GOOGLE_APPLICATION_CREDENTIALS: Path to a credentials file for interacting with
    GCP Cloud Monitoring services.
SCRIPTS_DIR: Path containing scripts for installing/configuring the various
    applications and agents. Also has some files that aren't technically
    scripts that tell the test what to do, such as supported_applications.txt.
FILTER: An optional Cloud Monitoring filter to use when querying for updated
    metrics descriptors. If omitted, the script will pull all metric descriptors
	using the following default filters:
		metric.type = starts_with("workload.googleapis.com/")
		metric.type = starts_with("agent.googleapis.com/iis/")
		metric.type = starts_with("agent.googleapis.com/mssql/")
	FILTER is useful when testing a single integration, for example,
		FILTER='metric.type=starts_with("workload.googleapis.com/apache")'
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"regexp"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/common"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	"go.uber.org/multierr"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/api/metric"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"gopkg.in/yaml.v2"
)

var (
	monClient  *monitoring.MetricClient
	project    = os.Getenv("PROJECT")
	scriptsDir = os.Getenv("SCRIPTS_DIR")
	filter     = os.Getenv("FILTER")
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	allMetrics, err := listAllMetrics(ctx, project)
	if err != nil {
		return err
	}

	metricsByApp := expectedMetricsByApp(allMetrics)
	for app, newMetrics := range metricsByApp {
		log.Printf("Processing %d metrics for %s...\n", len(newMetrics), app)
		metricsToWrite := make([]common.ExpectedMetric, 0)
		existingMetrics, readErr := readExpectedMetrics(app)
		if readErr != nil {
			err = multierr.Append(err, readErr)
			continue
		}
		// Write existing metrics first, updating them if needed
		for _, existingMetric := range existingMetrics {
			if newMetric := findMetric(newMetrics, existingMetric.Type); newMetric != nil {
				updateMetric(&existingMetric, newMetric)
			}
			metricsToWrite = append(metricsToWrite, existingMetric)
		}
		// Write exclusively new metrics last
		for _, newMetric := range newMetrics {
			if findMetric(existingMetrics, newMetric.Type) == nil {
				metricsToWrite = append(metricsToWrite, newMetric)
			}
		}
		err = multierr.Append(err, writeExpectedMetrics(app, metricsToWrite))
	}
	return err
}

// listMetrics calls projects.metricDescriptors.list with the given project ID and filter.
func listMetrics(ctx context.Context, project string, filter string) ([]*metric.MetricDescriptor, error) {
	req := &monitoringpb.ListMetricDescriptorsRequest{
		Name:   "projects/" + project + "/metricDescriptors/",
		Filter: filter,
	}
	it := monClient.ListMetricDescriptors(ctx, req)
	metrics := make([]*metric.MetricDescriptor, 0)
	for {
		m, err := it.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// listAllMetrics calls projects.metricDescriptors.list with the given project ID
// using the Cloud Monitoring filter defined in FILTER, or an exhaustive set of
// default filters if FILTER is not defined.
func listAllMetrics(ctx context.Context, project string) ([]*metric.MetricDescriptor, error) {
	metrics := make([]*metric.MetricDescriptor, 0)
	var err error
	var filters []string
	// User-defined FILTER takes priority over default filters
	if len(filter) > 0 {
		filters = []string{filter}
	} else {
		filters = []string{
			`metric.type = starts_with("workload.googleapis.com/")`,
			`metric.type = starts_with("agent.googleapis.com/iis/")`,
			`metric.type = starts_with("agent.googleapis.com/mssql/")`,
		}
	}
	for _, filter := range filters {
		listMetricsResult, listMetricsErr := listMetrics(ctx, project, filter)
		if listMetricsErr != nil {
			err = multierr.Append(err, listMetricsErr)
			continue
		}
		metrics = append(metrics, listMetricsResult...)
	}
	return metrics, err
}

// expectedMetricsByApp creates a map of the given metrics keyed on their
// respective app (e.g. apache, iis, etc.), converted to []ExpectedMetric.
func expectedMetricsByApp(metrics []*metric.MetricDescriptor) map[string][]common.ExpectedMetric {
	byApp := make(map[string][]common.ExpectedMetric, 0)
	for _, m := range metrics {
		matches := regexp.MustCompile(`.*\.googleapis.com\/([^/.]*)[/.].*`).FindStringSubmatch(m.Type)
		if len(matches) != 2 {
			panic(fmt.Errorf("metric type doesn't match regex: %s", m.Type))
		}
		app := matches[1]
		if app == "" {
			panic(fmt.Errorf("app not detected for: %s", m.Type))
		}
		byApp[app] = append(byApp[app], toExpectedMetric(m))
	}
	return byApp
}

// toExpectedMetric converts from metric.MetricDescriptor to ExpectedMetric.
func toExpectedMetric(metric *metric.MetricDescriptor) common.ExpectedMetric {
	labels := make(map[string]string, 0)
	for _, l := range metric.Labels {
		labels[l.Key] = ".*"
	}
	return common.ExpectedMetric{
		Type:              metric.Type,
		Kind:              metric.MetricKind.String(),
		ValueType:         metric.ValueType.String(),
		MonitoredResource: "gce_instance",
		Labels:            labels,
	}
}

func expectedMetricsFilename(app string) string {
	return path.Join(scriptsDir, "applications", app, "expected_metrics.yaml")
}

// readExpectedMetrics reads in the existing expected_metrics.yaml
// file for the given app. If none exist, an empty slice is returned.
// Otherwise, its contents are returned, or an error if it could
// not be unmarshaled.
func readExpectedMetrics(app string) ([]common.ExpectedMetric, error) {
	file := expectedMetricsFilename(app)
	serialized, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return make([]common.ExpectedMetric, 0), nil
	} else if err != nil {
		return nil, err
	}
	var metrics []common.ExpectedMetric
	if err = yaml.Unmarshal(serialized, &metrics); err != nil {
		return nil, err
	}
	return metrics, nil
}

// writeExpectedMetrics writes the given list of metrics to the
// expected_metrics.yaml associated with the given app.
func writeExpectedMetrics(app string, metrics []common.ExpectedMetric) error {
	serialized, err := yaml.Marshal(metrics)
	if err != nil {
		return err
	}
	file := expectedMetricsFilename(app)
	return os.WriteFile(file, serialized, 0644)
}

// updateMetric updates the given metric in-place using values from withValuesFrom.
// Existing Optional and Representative values are preserved, as well as existing
// label patterns. All other values are copied from withValuesFrom. Existing label
// keys not present in withValuesFrom.Labels are dropped.
func updateMetric(toUpdate *common.ExpectedMetric, withValuesFrom *common.ExpectedMetric) {
	if toUpdate.Type != withValuesFrom.Type {
		panic(fmt.Errorf("updateMetric: attempted to update metric with mismatched type: %s, %s", toUpdate.Type, withValuesFrom.Type))
	}
	toUpdate.Kind = withValuesFrom.Kind
	toUpdate.ValueType = withValuesFrom.ValueType
	toUpdate.MonitoredResource = withValuesFrom.MonitoredResource

	// TODO: Refactor to a simple map copy once we improve listMetrics to fetch
	// label patterns automatically.

	// Copy new label keys
	for k, v := range withValuesFrom.Labels {
		// Don't overwrite existing patterns
		if _, ok := toUpdate.Labels[k]; !ok {
			toUpdate.Labels[k] = v
		}
	}
	// Remove dropped label keys
	for k := range toUpdate.Labels {
		if _, ok := withValuesFrom.Labels[k]; !ok {
			delete(toUpdate.Labels, k)
		}
	}
}

func findMetric(existingMetrics []common.ExpectedMetric, metricType string) *common.ExpectedMetric {
	for _, existingMetric := range existingMetrics {
		if existingMetric.Type == metricType {
			return &existingMetric
		}
	}
	return nil
}

func init() {
	ctx := context.Background()
	var err error
	monClient, err = monitoring.NewMetricClient(ctx)
	if err != nil {
		panic(fmt.Errorf("NewMetricClient() failed: %v", err))
	}
}
