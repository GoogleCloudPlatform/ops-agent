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
)

// An implementation of the Resource interface will have fields represent
// available attributes about the current monitoring resource.
type Resource interface {
	GetType() string
	OTelResourceAttributes() map[string]string
}

// Get a resource instance for the current environment;
// In order to access the attributes of a specific type of resource,
// needs to cast the returned Resource instance to its underlying type:
// actual, ok := resource.(GCEResource)
func GetResource() (Resource, error) {
	switch {
	case OnBMS():
		return GetBMSResource()
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

func (UnrecognizedPlatformResource) GetType() string {
	return "unrecognized platform"
}

func (UnrecognizedPlatformResource) OTelResourceAttributes() map[string]string {
	return map[string]string{
		"cloud.platform": "unrecognized platform",
	}
}

func GetUnrecognizedPlatformResource() (Resource, error) {
	return UnrecognizedPlatformResource{}, nil
}
