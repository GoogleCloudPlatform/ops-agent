//go:build integration_test

/*
PROJECT: What GCP project to use.
GOOGLE_APPLICATION_CREDENTIALS: Path to a credentials file for interacting with
    some GCP services. All gcloud commands actually use a different set of
    credentials, those in CLOUDSDK_CONFIG (unfortunately).
SCRIPTS_DIR: a path containing scripts for installing/configuring the various
    applications and agents. Also has some files that aren't technically
    scripts that tell the test what to do, such as supported_applications.txt.
FILTER: an optional Cloud Monitoring filter to use when querying for updated
    metrics descriptors. If omitted, the script will pull all metric descriptors
	using the following default filters:
		metric.type = starts_with("workload")
		metric.type = starts_with("agent.googleapis.com/iis")
		metric.type = starts_with("agent.googleapis.com/mssql")
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

type expectedMetric struct {
	Type              string            `yaml:"type"`
	ValueType         string            `yaml:"value_type"`
	Kind              string            `yaml:"kind"`
	MonitoredResource string            `yaml:"monitored_resource"`
	Labels            map[string]string `yaml:"labels"`
	Optional          bool              `yaml:"optional,omitempty"`
	Representative    bool              `yaml:"representative,omitempty"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	listMetrics, err := listAllMetricsWithLabels(ctx, project)
	if err != nil {
		return err
	}

	metricsByApp := expectedMetricsByApp(listMetrics)
	for app, newMetrics := range metricsByApp {
		log.Printf("Processing %d metrics for %s...\n", len(newMetrics), app)
		metricsToWrite := make([]expectedMetric, 0)
		existingMetrics, readErr := readExpectedMetrics(app)
		if readErr != nil {
			err = multierr.Append(err, readErr)
			continue
		}
		// Write existing metrics first, updating them if needed
		for _, existingMetric := range existingMetrics {
			if newMetric := findMetric(newMetrics, existingMetric.Type); newMetric != nil {
				existingMetric = mergeMetric(existingMetric, *newMetric)
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
// Note that the returned descriptors do not include labels;
// a separate call to getMetricDescriptor is needed for that.
func listMetrics(ctx context.Context, project string, filter string) ([]*metric.MetricDescriptor, error) {
	req := &monitoringpb.ListMetricDescriptorsRequest{
		Name:   "projects/" + project + "/metricDescriptors/",
		Filter: filter,
	}
	it := monClient.ListMetricDescriptors(ctx, req)
	metrics := make([]*metric.MetricDescriptor, 0)
	for {
		metric, err := it.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
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
			`metric.type = starts_with("workload")`,
			`metric.type = starts_with("agent.googleapis.com/iis")`,
			`metric.type = starts_with("agent.googleapis.com/mssql")`,
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

// listAllMetricsWithLabels calls listAllMetrics, and for each result,
// subsequently calls projects.metricDescriptors.get to get label
// information as well.
func listAllMetricsWithLabels(ctx context.Context, project string) ([]*metric.MetricDescriptor, error) {
	metrics, err := listAllMetrics(ctx, project)
	if err != nil {
		return nil, err
	}
	for i, metric := range metrics {
		metricWithLabels, err := getMetricDescriptor(ctx, project, metric.Type)
		if err != nil {
			return nil, err
		}
		metrics[i] = metricWithLabels
	}
	return metrics, nil
}

// getMetricDescriptor gets all metric descriptor information,
// including labels, for a single metric type.
func getMetricDescriptor(ctx context.Context, project string, metricType string) (*metric.MetricDescriptor, error) {
	req := &monitoringpb.GetMetricDescriptorRequest{
		Name: "projects/" + project + "/metricDescriptors/" + metricType,
	}
	return monClient.GetMetricDescriptor(ctx, req)
}

// expectedMetricsByApp creates a map of the given metrics keyed on their
// respective app (e.g. apache, iis, etc.), converted to []expectedMetric.
func expectedMetricsByApp(metrics []*metric.MetricDescriptor) map[string][]expectedMetric {
	m := make(map[string][]expectedMetric, 0)
	for _, _metric := range metrics {
		matches := regexp.MustCompile(`.*\.googleapis.com\/([^/.]*)[/.].*`).FindStringSubmatch(_metric.Type)
		if len(matches) != 2 {
			panic(fmt.Errorf("metric type doesn't match regex: %s", _metric.Type))
		}
		app := matches[1]
		if app == "" {
			panic(fmt.Errorf("app not detected for: %s", _metric.Type))
		}
		existingMetrics, ok := m[app]
		if !ok {
			existingMetrics = make([]expectedMetric, 0)
		}
		existingMetrics = append(existingMetrics, toExpectedMetric(_metric))
		m[app] = existingMetrics
	}
	return m
}

// toExpectedMetric converts from metric.MetricDescriptor to expectedMetric.
func toExpectedMetric(metric *metric.MetricDescriptor) expectedMetric {
	labels := make(map[string]string, 0)
	for _, l := range metric.Labels {
		labels[l.Key] = ".*"
	}
	return expectedMetric{
		Type:              metric.Type,
		Kind:              metric.MetricKind.String(),
		ValueType:         metric.ValueType.String(),
		MonitoredResource: "gce_instance",
		Labels:            labels,
	}
}

// readExpectedMetrics reads in the existing expected_metrics.yaml
// file for the given app. If none exist, an empty slice is returned.
// Otherwise, its contents are returned, or an error if it could
// not be unmarshaled.
func readExpectedMetrics(app string) ([]expectedMetric, error) {
	file := path.Join(scriptsDir, "applications", app, "expected_metrics.yaml")
	serialized, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return make([]expectedMetric, 0), nil
	} else if err != nil {
		return nil, err
	}
	var metrics []expectedMetric
	if err = yaml.Unmarshal(serialized, &metrics); err != nil {
		return nil, err
	}
	return metrics, nil
}

// writeExpectedMetrics write the given list of metrics to the
// expected_metrics.yaml associated with the given app.
func writeExpectedMetrics(app string, metrics []expectedMetric) error {
	serialized, err := yaml.Marshal(metrics)
	if err != nil {
		return err
	}
	file := path.Join(scriptsDir, "applications", app, "expected_metrics.yaml")
	return os.WriteFile(file, serialized, 0644)
}

// mergeMetric produces a combination of the two given metrics, which
// is based on newMetric but with Optional, Representative, and Label
// patterns inherited from existingMetric.
func mergeMetric(existingMetric expectedMetric, newMetric expectedMetric) expectedMetric {
	merged := newMetric
	// Use existing Optional and Representative
	merged.Optional = existingMetric.Optional
	merged.Representative = existingMetric.Representative
	// Use any existing Label patterns
	for label, pattern := range existingMetric.Labels {
		if _, ok := merged.Labels[label]; ok {
			merged.Labels[label] = pattern
		}
	}
	return merged
}

func findMetric(existingMetrics []expectedMetric, metricType string) *expectedMetric {
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
