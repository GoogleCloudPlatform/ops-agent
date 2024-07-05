// Copyright 2022 Google LLC
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

package resourcedetector

import (
	gcp_metadata "cloud.google.com/go/compute/metadata"
	"google.golang.org/genproto/googleapis/api/monitoredres"
)

// An implementation of the Resource interface will have fields represent
// available attributes about the current monitoring resource.
type Resource interface {
	ProjectName() string
	MonitoredResource() *monitoredres.MonitoredResource
	OTelResourceAttributes() map[string]string
	PrometheusStyleMetadata() map[string]string
	ExtraLogLabels() map[string]string
}

type resourceCache struct {
	Resource Resource
	Error    error
}

var cachedResourceAndError *resourceCache

// Get a resource instance for the current environment;
// In order to access the attributes of a specific type of resource,
// needs to cast the returned Resource instance to its underlying type:
// actual, ok := resource.(GCEResource)
func GetResource() (Resource, error) {
	if cachedResourceAndError != nil {
		return cachedResourceAndError.Resource, cachedResourceAndError.Error
	}
	r, err := getUncachedResource()
	cachedResourceAndError = &resourceCache{
		Resource: r,
		Error:    err,
	}
	return r, err
}

func getUncachedResource() (Resource, error) {
	switch {
	case gcp_metadata.OnGCE():
		return GetGCEResource()
	default:
		return GetUnrecognizedPlatformResource()
	}
}

// UnrecognizedPlatformResource that returns an empty resource instance without any attributes
// for unrecognized environments
type UnrecognizedPlatformResource struct {
}

func (UnrecognizedPlatformResource) ProjectName() string {
	return ""
}

func (UnrecognizedPlatformResource) MonitoredResource() *monitoredres.MonitoredResource {
	return nil
}

func (UnrecognizedPlatformResource) OTelResourceAttributes() map[string]string {
	return nil
}

func (UnrecognizedPlatformResource) PrometheusStyleMetadata() map[string]string {
	return nil
}

func (UnrecognizedPlatformResource) ExtraLogLabels() map[string]string {
	return nil
}

func GetUnrecognizedPlatformResource() (Resource, error) {
	return UnrecognizedPlatformResource{}, nil
}
