// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build gpu
// +build gpu

package dcgmreceiver

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"go.uber.org/zap"
)

const maxWarningsForFailedDeviceMetricQuery = 5

const dcgmProfilingFieldsStart = dcgm.Short(1000)

var ErrDcgmInitialization = errors.New("error initializing DCGM")

type dcgmClientSettings struct {
	endpoint         string
	pollingInterval  time.Duration
	retryBlankValues bool
	maxRetries       int
	fields           []string
}

type deviceMetrics struct {
	ModelName string
	UUID      string
	Metrics   MetricsMap
}

type dcgmClient struct {
	logger            *zap.SugaredLogger
	handleCleanup     func()
	enabledFieldIDs   []dcgm.Short
	enabledFieldGroup dcgm.FieldHandle
	deviceGroup       dcgm.GroupHandle

	devices            map[uint]deviceMetrics
	lastSuccessfulPoll time.Time

	deviceMetricToFailedQueryCount map[string]int
	pollingInterval                time.Duration
	retryBlankValues               bool
	maxRetries                     int
}

// Can't pass argument dcgm.mode because it is unexported
var dcgmInit = func(args ...string) (func(), error) {
	return dcgm.Init(dcgm.Standalone, args...)
}

var dcgmGetValuesSince = dcgm.GetValuesSince

func newClient(settings *dcgmClientSettings, logger *zap.Logger) (*dcgmClient, error) {
	dcgmCleanup, err := initializeDcgm(settings.endpoint, logger)
	if err != nil {
		return nil, errors.Join(ErrDcgmInitialization, err)
	}
	enabledFieldGroup := dcgm.FieldHandle{}
	requestedFieldIDs := toFieldIDs(settings.fields)
	supportedProfilingFieldIDs, err := getSupportedProfilingFields()
	if err != nil {
		// If there is error querying the supported fields at all, let the
		// receiver collect basic metrics: (GPU utilization, used/free memory).
		logger.Sugar().Warnf("Error querying supported profiling fields on '%w'. GPU profiling metrics will not be collected.", err)
	}
	enabledFields, unavailableFields := filterSupportedFields(requestedFieldIDs, supportedProfilingFieldIDs)
	for _, f := range unavailableFields {
		logger.Sugar().Warnf("Field '%s' is not supported", dcgmIDToName[f])
	}
	var deviceGroup dcgm.GroupHandle
	if len(enabledFields) != 0 {
		supportedDeviceIndices, err := dcgm.GetSupportedDevices()
		if err != nil {
			return nil, fmt.Errorf("Unable to discover supported GPUs on %w", err)
		}
		logger.Sugar().Infof("Discovered %d supported GPU devices", len(supportedDeviceIndices))

		deviceGroup, err = createDeviceGroup(logger, supportedDeviceIndices)
		if err != nil {
			return nil, err
		}
		enabledFieldGroup, err = setWatchesOnEnabledFields(settings.pollingInterval, logger, deviceGroup, enabledFields)
		if err != nil {
			_ = dcgm.FieldGroupDestroy(enabledFieldGroup)
			return nil, fmt.Errorf("Unable to set field watches on %w", err)
		}
	}
	return &dcgmClient{
		logger:                         logger.Sugar(),
		handleCleanup:                  dcgmCleanup,
		enabledFieldIDs:                enabledFields,
		enabledFieldGroup:              enabledFieldGroup,
		deviceGroup:                    deviceGroup,
		devices:                        map[uint]deviceMetrics{},
		lastSuccessfulPoll:             time.Now(),
		deviceMetricToFailedQueryCount: make(map[string]int),
		pollingInterval:                settings.pollingInterval,
		retryBlankValues:               settings.retryBlankValues,
		maxRetries:                     settings.maxRetries,
	}, nil
}

// initializeDcgm tries to initialize a DCGM connection; returns a cleanup func
// only if the connection is initialized successfully without error
func initializeDcgm(endpoint string, logger *zap.Logger) (func(), error) {
	isSocket := "0"
	dcgmCleanup, err := dcgmInit(endpoint, isSocket)
	if err != nil {
		msg := fmt.Sprintf("Unable to connect to DCGM daemon at %s on %v; Is the DCGM daemon running?", endpoint, err)
		logger.Sugar().Warn(msg)
		if dcgmCleanup != nil {
			dcgmCleanup()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	logger.Sugar().Infof("Connected to DCGM daemon at %s", endpoint)
	return dcgmCleanup, nil
}

func newDeviceMetrics(logger *zap.SugaredLogger, gpuIndex uint) (deviceMetrics, error) {
	deviceInfo, err := dcgm.GetDeviceInfo(gpuIndex)
	if err != nil {
		logger.Warnf("Unable to query device info for NVIDIA device %d on '%w'", gpuIndex, err)
		return deviceMetrics{}, err
	}

	device := deviceMetrics{
		ModelName: deviceInfo.Identifiers.Model,
		UUID:      deviceInfo.UUID,
		Metrics:   MetricsMap{},
	}
	logger.Infof("Discovered NVIDIA device %s with UUID %s (DCGM GPU ID %d)", device.ModelName, device.UUID, gpuIndex)
	return device, nil
}

func createDeviceGroup(logger *zap.Logger, deviceIndices []uint) (dcgm.GroupHandle, error) {
	deviceGroupName := "google-cloud-ops-agent-group"
	deviceGroup, err := dcgm.CreateGroup(deviceGroupName)
	if err != nil {
		return dcgm.GroupHandle{}, fmt.Errorf("Unable to create DCGM GPU group '%s' on %w", deviceGroupName, err)
	}

	for _, gpuIndex := range deviceIndices {
		err = dcgm.AddToGroup(deviceGroup, gpuIndex)
		if err != nil {
			return dcgm.GroupHandle{}, fmt.Errorf("Unable add NVIDIA device %d to GPU group '%s' on %w", gpuIndex, deviceGroupName, err)
		}
	}

	logger.Sugar().Infof("Created GPU group '%s'", deviceGroupName)
	return deviceGroup, nil
}

func toFieldIDs(fields []string) []dcgm.Short {
	requestedFieldIDs := make([]dcgm.Short, len(fields))
	for i, f := range fields {
		requestedFieldIDs[i] = dcgm.DCGM_FI[f]
	}
	return requestedFieldIDs
}

// getSupportedProfilingFields calls the DCGM query function to find out all
// profiling fields that are supported by the current GPUs
func getSupportedProfilingFields() ([]dcgm.Short, error) {
	supported := []dcgm.Short{}
	// GetSupportedMetricGroups currently does not support passing the actual
	// group handle; here we pass 0 to query supported fields for group 0, which
	// is the default DCGM group that is **supposed** to include all GPUs of the
	// host.
	fieldGroups, err := dcgm.GetSupportedMetricGroups(0)
	if err != nil {
		var dcgmErr *dcgm.DcgmError
		if errors.As(err, &dcgmErr) {
			// When the device does not support profiling metrics, this function
			// will return DCGM_ST_MODULE_NOT_LOADED:
			// "This request is serviced by a module of DCGM that is not
			// currently loaded." Example of this is NVIDIA P4
			if dcgmErr.Code == dcgm.DCGM_ST_MODULE_NOT_LOADED {
				return supported, nil
			}
		}
		return supported, err
	}
	for i := 0; i < len(fieldGroups); i++ {
		for j := 0; j < len(fieldGroups[i].FieldIds); j++ {
			supported = append(supported, dcgm.Short(fieldGroups[i].FieldIds[j]))
		}
	}
	return supported, nil
}

// filterSupportedFields takes the user requested fields and device supported
// profiling fields, and filters to return those that are requested & supported
// to be the enabledFields and requested but not supported as unavailableFields
func filterSupportedFields(requestedFields []dcgm.Short, supportedProfilingFields []dcgm.Short) ([]dcgm.Short, []dcgm.Short) {
	var enabledFields []dcgm.Short
	var unavailableFields []dcgm.Short
	for _, ef := range requestedFields {
		// For fields like `DCGM_FI_DEV_*`, which are not
		// profiling fields, assume they are always present.
		support := ef < dcgmProfilingFieldsStart
		for _, sf := range supportedProfilingFields {
			if sf == ef {
				support = true
				break
			}
		}
		if support {
			enabledFields = append(enabledFields, ef)
		} else {
			unavailableFields = append(unavailableFields, ef)
		}
	}
	return enabledFields, unavailableFields
}

// Internal-only
type dcgmWatchParams struct {
	fieldGroupName string
	updateFreqUs   int64
	maxKeepTime    float64
	maxKeepSamples int32
}

// Internal-only
func setWatchesOnFields(logger *zap.Logger, deviceGroup dcgm.GroupHandle, fieldIDs []dcgm.Short, params dcgmWatchParams) (dcgm.FieldHandle, error) {
	var err error

	fieldGroup, err := dcgm.FieldGroupCreate(params.fieldGroupName, fieldIDs)
	if err != nil {
		return dcgm.FieldHandle{}, fmt.Errorf("Unable to create DCGM field group '%s'", params.fieldGroupName)
	}

	msg := fmt.Sprintf("Created DCGM field group '%s' with field ids: ", params.fieldGroupName)
	for _, fieldID := range fieldIDs {
		msg += fmt.Sprintf("%d ", fieldID)
	}
	logger.Sugar().Info(msg)

	// Note: DCGM retained samples = Max(maxKeepSamples, maxKeepTime/updateFreq)
	dcgmUpdateFreq := params.updateFreqUs
	dcgmMaxKeepTime := params.maxKeepTime
	dcgmMaxKeepSamples := params.maxKeepSamples
	err = dcgm.WatchFieldsWithGroupEx(fieldGroup, deviceGroup, dcgmUpdateFreq, dcgmMaxKeepTime, dcgmMaxKeepSamples)
	if err != nil {
		return fieldGroup, fmt.Errorf("Setting watches for DCGM field group '%s' failed on %w", params.fieldGroupName, err)
	}
	logger.Sugar().Infof("Setting watches for DCGM field group '%s' succeeded", params.fieldGroupName)

	return fieldGroup, nil
}

const maxKeepSamples = 100 // TODO: Is this enough?

func setWatchesOnEnabledFields(pollingInterval time.Duration, logger *zap.Logger, deviceGroup dcgm.GroupHandle, enabledFieldIDs []dcgm.Short) (dcgm.FieldHandle, error) {
	return setWatchesOnFields(logger, deviceGroup, enabledFieldIDs, dcgmWatchParams{
		// Note: Add random suffix to avoid conflict amongnst any parallel collectors
		fieldGroupName: fmt.Sprintf("google-cloud-ops-agent-metrics-%d", randSource.Intn(10000)),
		// Note: DCGM retained samples = Max(maxKeepSamples, maxKeepTime/updateFreq)
		updateFreqUs:   int64(pollingInterval / time.Microsecond),
		maxKeepTime:    600.0, /* 10 min */
		maxKeepSamples: maxKeepSamples,
	})
}

func (client *dcgmClient) cleanup() {
	_ = dcgm.FieldGroupDestroy(client.enabledFieldGroup)
	_ = dcgm.DestroyGroup(client.deviceGroup)
	if client.handleCleanup != nil {
		client.handleCleanup()
	}

	client.logger.Info("Shutdown DCGM")
}

// collect will poll dcgm for any new metrics, updating client.devices as appropriate
// It returns the estimated polling interval.
func (client *dcgmClient) collect() (time.Duration, error) {
	client.logger.Debugf("Polling DCGM daemon for field values")
	if len(client.enabledFieldIDs) == 0 {
		// Make sure we don't try to scrape without a device group (since we don't construct one when there are no enabled fields).
		return 0, nil
	}
	fieldValues, pollTime, err := dcgmGetValuesSince(client.deviceGroup, client.enabledFieldGroup, client.lastSuccessfulPoll)
	if err != nil {
		msg := fmt.Sprintf("Unable to poll DCGM daemon for metrics: %s", err)
		client.issueWarningForFailedQueryUptoThreshold("all-profiling-metrics", maxWarningsForFailedDeviceMetricQuery, msg)
		return 0, err
	}
	client.logger.Debugf("Got %d field values over %s", len(fieldValues), pollTime.Sub(client.lastSuccessfulPoll))
	client.lastSuccessfulPoll = pollTime
	oldestTs := int64(math.MaxInt64)
	newestTs := int64(0)
	for _, fieldValue := range fieldValues {
		if fieldValue.EntityGroupId != dcgm.FE_GPU {
			continue
		}
		gpuIndex := fieldValue.EntityId
		if _, ok := client.devices[gpuIndex]; !ok {
			device, err := newDeviceMetrics(client.logger, gpuIndex)
			if err != nil {
				continue
			}
			client.devices[gpuIndex] = device
		}
		device := client.devices[gpuIndex]
		dcgmName := dcgmIDToName[dcgm.Short(fieldValue.FieldId)]
		if err := isValidValue(fieldValue); err == errBlankValue {
			// Blank values are expected at startup.
			continue
		} else if err == errNotSupported {
			client.issueWarningForFailedQueryUptoThreshold(dcgmName, 1, fmt.Sprintf("Field '%s' is not supported", dcgmName))
			continue
		} else if err != nil {
			msg := fmt.Sprintf("Received invalid value (ts %d gpu %d) %s: %v", fieldValue.Ts, gpuIndex, dcgmName, err)
			client.issueWarningForFailedQueryUptoThreshold(fmt.Sprintf("device%d.%s", gpuIndex, dcgmName), maxWarningsForFailedDeviceMetricQuery, msg)
			continue
		}
		if fieldValue.Ts < oldestTs {
			oldestTs = fieldValue.Ts
		}
		if fieldValue.Ts > newestTs {
			newestTs = fieldValue.Ts
		}
		if _, ok := device.Metrics[dcgmName]; !ok {
			device.Metrics[dcgmName] = &metricStats{}
		}
		device.Metrics[dcgmName].Update(fieldValue)
	}
	duration := time.Duration(newestTs-oldestTs) * time.Microsecond
	client.logger.Debugf("Successful poll of DCGM daemon returned %v of data", duration)
	// If we did a partial poll, there should be more room in the buffer.
	duration = max(duration, client.pollingInterval*maxKeepSamples)
	return duration, nil
}

// getDeviceMetrics returns a deep copy of client.devices
func (client *dcgmClient) getDeviceMetrics() map[uint]deviceMetrics {
	out := map[uint]deviceMetrics{}
	for gpuIndex, device := range client.devices {
		newMetrics := MetricsMap{}
		for key, value := range device.Metrics {
			newValue := *value
			newMetrics[key] = &newValue
		}
		// device is already a copy here
		device.Metrics = newMetrics
		out[gpuIndex] = device
	}
	return out
}

func (client *dcgmClient) issueWarningForFailedQueryUptoThreshold(dcgmName string, limit int, reason string) {
	client.deviceMetricToFailedQueryCount[dcgmName]++

	failedCount := client.deviceMetricToFailedQueryCount[dcgmName]
	if failedCount <= limit {
		client.logger.Warn(reason)
		if limit > 1 && failedCount == limit {
			client.logger.Warnf("Surpressing further device query warnings for '%s'", dcgmName)
		}
	}
}
