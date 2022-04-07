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
	using a set of default filters; see the defaultFilters variable.
	FILTER is useful when testing a single integration, for example,
		FILTER='metric.type=starts_with("workload.googleapis.com/apache")'
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
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
	monClient      *monitoring.MetricClient
	project        = os.Getenv("PROJECT")
	scriptsDir     = os.Getenv("SCRIPTS_DIR")
	filter         = os.Getenv("FILTER")
	defaultFilters = []string{
		`metric.type = starts_with("workload.googleapis.com/")`,
		`metric.type = starts_with("agent.googleapis.com/iis/")`,
		`metric.type = starts_with("agent.googleapis.com/mssql/")`,
	}
)

type expectedMetricsMap map[string]common.ExpectedMetric

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	allMetrics, err := listAllMetricsByApp(ctx, project)
	if err != nil {
		return err
	}

	for app, newMetrics := range allMetrics {
		log.Printf("Processing %d metrics for %s...\n", len(newMetrics), app)
		existingMetrics, readErr := readExpectedMetrics(app)
		if readErr != nil {
			err = multierr.Append(err, readErr)
			continue
		}
		// For each new metric, either update the corresponding existing metric,
		// or add it.
		for _, newMetric := range newMetrics {
			existingMetrics[newMetric.Type] = updateMetric(existingMetrics[newMetric.Type], newMetric)
		}
		err = multierr.Append(err, writeExpectedMetrics(app, existingMetrics))
	}
	return err
}

// listMetrics calls projects.metricDescriptors.list with the given project ID and filter.
func listMetrics(ctx context.Context, project string, filter string) ([]metric.MetricDescriptor, error) {
	req := &monitoringpb.ListMetricDescriptorsRequest{
		Name:   "projects/" + project + "/metricDescriptors/",
		Filter: filter,
	}
	it := monClient.ListMetricDescriptors(ctx, req)
	metrics := make([]metric.MetricDescriptor, 0)
	for {
		m, err := it.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		metrics = append(metrics, *m)
	}
	return metrics, nil
}

// listAllMetrics calls projects.metricDescriptors.list with the given project ID
// using the Cloud Monitoring filter defined in FILTER, or an exhaustive set of
// default filters if FILTER is not defined. Metrics are returned as a map from
// app name to expectedMetricsMap.
func listAllMetricsByApp(ctx context.Context, project string) (map[string]expectedMetricsMap, error) {
	metrics := make(map[string]expectedMetricsMap)
	var err error
	var filters []string
	// User-defined FILTER takes priority over default filters
	if len(filter) > 0 {
		filters = []string{filter}
	} else {
		filters = defaultFilters
	}
	for _, filter := range filters {
		listMetricsResult, listMetricsErr := listMetrics(ctx, project, filter)
		if listMetricsErr != nil {
			err = multierr.Append(err, listMetricsErr)
			continue
		}
		for _, m := range listMetricsResult {
			app := getAppName(m.Type)
			if _, ok := metrics[app]; !ok {
				metrics[app] = make(expectedMetricsMap)
			} else if _, ok := metrics[app][m.Type]; ok {
				err = multierr.Append(err, fmt.Errorf("duplicate metric found, skipping: %s", m.Type))
				continue
			}
			metrics[app][m.Type] = toExpectedMetric(m)
		}
	}
	return metrics, err
}

// getAppName parses out the app name from a metric type, for example:
//   workload.googleapis.com/apache.xyz -> apache
//   agent.googleapis.com/iis/xyz -> iis
func getAppName(metricType string) string {
	matches := regexp.MustCompile(`.*\.googleapis.com\/([^/.]*)[/.].*`).FindStringSubmatch(metricType)
	if len(matches) != 2 {
		panic(fmt.Errorf("metric type doesn't match regex: %s", metricType))
	}
	app := matches[1]
	if app == "" {
		panic(fmt.Errorf("app not detected for metric type: %s", metricType))
	}
	return app
}

// toExpectedMetric converts from metric.MetricDescriptor to ExpectedMetric.
func toExpectedMetric(metric metric.MetricDescriptor) common.ExpectedMetric {
	labels := make(map[string]string)
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

func metadataFilename(app string) string {
	return path.Join(scriptsDir, "applications", app, "metadata.yaml")
}

func readMetadata(app string) (common.IntegrationMetadata, error) {
	file := metadataFilename(app)
	serialized, err := os.ReadFile(file)
	var metadata common.IntegrationMetadata
	if err != nil {
		return metadata, err
	}
	err = yaml.Unmarshal(serialized, &metadata)
	return metadata, err
}

// readExpectedMetrics reads in metrics from the existing metadata.yaml
// file for the given app as a map keyed on metric type. If no metrics
// exist, an empty map is returned.
// Otherwise, its contents are returned, or an error if it could
// not be unmarshaled.
func readExpectedMetrics(app string) (expectedMetricsMap, error) {
	metadata, err := readMetadata(app)
	if err != nil {
		return nil, err
	}
	metricsByType := make(expectedMetricsMap)
	expectedMetrics := metadata.ExpectedMetrics
	for _, m := range expectedMetrics {
		m := m
		if _, ok := metricsByType[m.Type]; ok {
			return nil, fmt.Errorf("duplicate expected_metrics type in %s/metadata.yaml: %s", app, m.Type)
		}
		metricsByType[m.Type] = m
	}
	return metricsByType, nil
}

// writeExpectedMetrics writes the given map's values as a slice
// to the metadata.yaml associated with the given app. Metrics
// are written in alphabetical order by type.
func writeExpectedMetrics(app string, metrics expectedMetricsMap) error {
	metadata, err := readMetadata(app)
	if err != nil {
		return err
	}
	expectedMetrics := make([]common.ExpectedMetric, 0)
	for _, m := range metrics {
		expectedMetrics = append(expectedMetrics, m)
	}
	sort.Slice(expectedMetrics, func(i, j int) bool { return expectedMetrics[i].Type < expectedMetrics[j].Type })
	metadata.ExpectedMetrics = expectedMetrics
	serialized, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(metadataFilename(app), serialized, 0644)
}

// updateMetric returns the given metric with updates applied from withValuesFrom.
// Existing Optional and Representative values are preserved, as well as existing
// label patterns. All other values are copied from withValuesFrom. Existing label
// keys not present in withValuesFrom.Labels are dropped.
// If toUpdate.Type is empty, then withValuesFrom is returned.
func updateMetric(toUpdate common.ExpectedMetric, withValuesFrom common.ExpectedMetric) common.ExpectedMetric {
	if toUpdate.Type == "" {
		// Empty struct to update; just copy over the new one
		return withValuesFrom
	}
	if toUpdate.Type != withValuesFrom.Type {
		panic(fmt.Errorf("updateMetric: attempted to update metric with mismatched type: %s, %s", toUpdate.Type, withValuesFrom.Type))
	}
	result := toUpdate
	result.Kind = withValuesFrom.Kind
	result.ValueType = withValuesFrom.ValueType
	result.MonitoredResource = withValuesFrom.MonitoredResource
	result.Labels = make(map[string]string)

	// TODO: Refactor to a simple map copy once we improve listMetrics to fetch
	// label patterns automatically.

	// The keys of result.Labels should be the same as withValuesFrom.Labels,
	// except that existing values/patterns are preserved.
	for k, v := range withValuesFrom.Labels {
		existingPattern, ok := toUpdate.Labels[k]
		if ok {
			result.Labels[k] = existingPattern
		} else {
			result.Labels[k] = v
		}
	}

	return result
}

func init() {
	ctx := context.Background()
	var err error
	monClient, err = monitoring.NewMetricClient(ctx)
	if err != nil {
		panic(fmt.Errorf("NewMetricClient() failed: %v", err))
	}
}
