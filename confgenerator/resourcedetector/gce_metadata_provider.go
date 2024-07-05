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
	"fmt"
	"strings"

	gcp_metadata "cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp"
)

const notAvailable = "NOT AVAILABLE"

// Provide the GCE metadata using the on-VM metadata server
// The following metadata are not available on the metadata server:
// * Labels
// * Subnet URL
type GCEMetadataProvider struct {
	client *gcp_metadata.Client
}

func NewGCEMetadataProvider() gceDataProvider {
	c := gcp_metadata.NewClient(nil)
	return &GCEMetadataProvider{c}
}

func (gmp *GCEMetadataProvider) getProject() (string, error) {
	return gmp.client.ProjectID()
}

func (gmp *GCEMetadataProvider) getZone() (string, error) {
	return gmp.client.Zone()
}

// We assume the current running instance has at least one network interface
// Otherwise won't be able to connect to
func (gmp *GCEMetadataProvider) getNetwork() (string, error) {
	return gmp.client.Get("instance/network-interfaces/0/network")
}

// TODO: b/246995894
// The subnetwork url is currently not available from the metadata server
func (gmp *GCEMetadataProvider) getSubnetwork() (string, error) {
	return notAvailable, nil
}

func (gmp *GCEMetadataProvider) getPublicIP() (string, error) {
	return gmp.client.ExternalIP()
}

func (gmp *GCEMetadataProvider) getPrivateIP() (string, error) {
	return gmp.client.InternalIP()
}

func (gmp *GCEMetadataProvider) getInstanceID() (string, error) {
	return gmp.client.InstanceID()
}

func (gmp *GCEMetadataProvider) getInstanceName() (string, error) {
	return gmp.client.InstanceName()
}

func (gmp *GCEMetadataProvider) getTags() (string, error) {
	tags, err := gmp.client.InstanceTags()
	if err != nil {
		return "", err
	}
	return strings.Join(tags, ","), nil
}

func (gmp *GCEMetadataProvider) getMachineType() (string, error) {
	return gmp.client.Get("instance/machine-type")
}

func (gmp *GCEMetadataProvider) getDefaultScopes() ([]string, error) {
	scopes, err := gmp.client.Scopes("default")
	// If default service account is not defined return empty scopes
	if _, ok := err.(gcp_metadata.NotDefinedError); ok {
		return []string{}, nil
	}
	return scopes, err
}

func (gmp *GCEMetadataProvider) getMetadata() (map[string]string, error) {
	keys, err := gmp.client.Get("instance/attributes")
	if err != nil {
		return map[string]string{}, err
	}
	metadata := map[string]string{}
	for _, key := range strings.Fields(keys) {
		val, err := gmp.client.Get(fmt.Sprintf("instance/attributes/%s", key))
		if err != nil {
			return map[string]string{}, err
		}
		metadata[key] = val
	}
	return metadata, nil
}

// TODO: b/246995462
// GCE VM labels are currently not available in the metadata server
func (gmp *GCEMetadataProvider) getLabels() (map[string]string, error) {
	return map[string]string{}, nil
}

func (gmp *GCEMetadataProvider) getInterfaceIPv4s() (map[string]string, error) {
	names, err := gmp.client.Get("instance/network-interfaces/")
	if err != nil {
		return map[string]string{}, err
	}
	interfaces := map[string]string{}
	for _, name := range strings.Fields(names) {
		// The metadata server would return interfaces as "0/", needs to trim the "/"
		name = strings.TrimRight(name, "/")
		ip, err := gmp.client.Get(fmt.Sprintf("instance/network-interfaces/%s/ip", name))
		if err != nil {
			return map[string]string{}, err
		}
		interfaces[name] = ip
	}
	return interfaces, err
}

func (gmp *GCEMetadataProvider) getMIG() (gcp.ManagedInstanceGroup, error) {
	return gcp.NewDetector().GCEManagedInstanceGroup()
}
