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

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp"
	"github.com/prometheus/prometheus/util/strutil"
	"google.golang.org/genproto/googleapis/api/monitoredres"
)

type gceAttribute int

const (
	project gceAttribute = iota
	zone
	network
	subnetwork
	publicIP
	privateIP
	instanceID
	instanceName
	tags
	machineType
	metadata
	label
	interfaceIPv4
	defaultScopes
)

func GetGCEResource() (Resource, error) {
	provider := NewGCEMetadataProvider()
	dt := GCEResourceBuilder{provider: provider}
	return dt.GetResource()
}

// The data provider interface for GCE environment
// Implementation of this provider can use either the metadata server on VM,
// or the cloud API
type gceDataProvider interface {
	getProject() (string, error)
	getZone() (string, error)
	getNetwork() (string, error)
	getSubnetwork() (string, error)
	getPublicIP() (string, error)
	getPrivateIP() (string, error)
	getInstanceID() (string, error)
	getInstanceName() (string, error)
	getTags() (string, error)
	getMachineType() (string, error)
	getDefaultScopes() ([]string, error)
	getLabels() (map[string]string, error)
	getMetadata() (map[string]string, error)
	getInterfaceIPv4s() (map[string]string, error)
	getMIG() (gcp.ManagedInstanceGroup, error)
}

// List of single-valued attributes (non-nested)
var singleAttributeSpec = map[gceAttribute]func(gceDataProvider) (string, error){
	project:      gceDataProvider.getProject,
	zone:         gceDataProvider.getZone,
	network:      gceDataProvider.getNetwork,
	subnetwork:   gceDataProvider.getSubnetwork,
	publicIP:     gceDataProvider.getPublicIP,
	privateIP:    gceDataProvider.getPrivateIP,
	instanceID:   gceDataProvider.getInstanceID,
	instanceName: gceDataProvider.getInstanceName,
	tags:         gceDataProvider.getTags,
	machineType:  gceDataProvider.getMachineType,
}

// List of multi-valued attributes (non-nested)
var multiAttributeSpec = map[gceAttribute]func(gceDataProvider) ([]string, error){
	defaultScopes: gceDataProvider.getDefaultScopes,
}

// List of nested attributes
var nestedAttributeSpec = map[gceAttribute]func(gceDataProvider) (map[string]string, error){
	metadata:      gceDataProvider.getMetadata,
	interfaceIPv4: gceDataProvider.getInterfaceIPv4s,
	label:         gceDataProvider.getLabels,
}

// GCEResource implements the Resource interface and provide attributes of the VM when on GCE
type GCEResource struct {
	Project              string
	Zone                 string
	Network              string
	Subnetwork           string
	PublicIP             string
	PrivateIP            string
	InstanceID           string
	InstanceName         string
	Tags                 string
	MachineType          string
	DefaultScopes        []string
	Metadata             map[string]string
	Label                map[string]string
	InterfaceIPv4        map[string]string
	ManagedInstanceGroup gcp.ManagedInstanceGroup
}

func (r GCEResource) ProjectName() string {
	return r.Project
}

func (r GCEResource) OTelResourceAttributes() map[string]string {
	return map[string]string{
		"cloud.platform":          "gcp_compute_engine",
		"cloud.project":           r.Project,
		"cloud.availability_zone": r.Zone,
		"cloud.region":            r.Zone,
		"host.id":                 r.InstanceID,
	}
}

func (r GCEResource) MonitoredResource() *monitoredres.MonitoredResource {
	return &monitoredres.MonitoredResource{
		Type: "gce_instance",
		Labels: map[string]string{
			"instance_id": r.InstanceID,
			"zone":        r.Zone,
		},
	}
}

func (r GCEResource) PrometheusStyleMetadata() map[string]string {
	metaLabels := map[string]string{
		"__meta_gce_instance_id":   r.InstanceID,
		"__meta_gce_instance_name": r.InstanceName,
		"__meta_gce_project":       r.Project,
		"__meta_gce_zone":          r.Zone,
		"__meta_gce_network":       r.Network,
		// TODO(b/b/246995894): Add support for subnetwork label.
		// "__meta_gce_subnetwork":    r.Subnetwork,
		"__meta_gce_public_ip":    r.PublicIP,
		"__meta_gce_private_ip":   r.PrivateIP,
		"__meta_gce_tags":         r.Tags,
		"__meta_gce_machine_type": r.MachineType,
	}
	prefix := "__meta_gce_"
	for k, v := range r.Metadata {
		sanitizedKey := "metadata_" + strutil.SanitizeLabelName(k)
		// Once https://github.com/open-telemetry/opentelemetry-collector/issues/9204
		// is fixed, this will no longer be needed.
		sanitizedVal := strings.ReplaceAll(v, "${", "_{")
		sanitizedVal = strings.ReplaceAll(sanitizedVal, "$", "$$")
		metaLabels[prefix+sanitizedKey] = sanitizedVal

	}

	// Labels are not available using the GCE metadata API.
	// TODO(b/246995462): Add support for labels.
	//
	// for k, v := range r.Label {
	// 	metaLabels[prefix+"label_"+k] = v
	// }

	for k, v := range r.InterfaceIPv4 {
		sanitizedKey := "interface_ipv4_nic" + strutil.SanitizeLabelName(k)
		metaLabels[prefix+sanitizedKey] = v
	}

	// Set the location, namespace and cluster labels.
	metaLabels["location"] = r.Zone
	metaLabels["namespace"] = fmt.Sprintf("%s/%s", r.InstanceID, r.InstanceName)
	metaLabels["cluster"] = "__gce__"

	// Set some curated labels.
	metaLabels["instance_name"] = r.InstanceName
	metaLabels["machine_type"] = r.MachineType

	return metaLabels
}

func (r GCEResource) ExtraLogLabels() map[string]string {
	l := make(map[string]string)
	if r.ManagedInstanceGroup.Name != "" {
		l[`compute.googleapis.com/instance_group_manager/name`] = r.ManagedInstanceGroup.Name
	}
	switch r.ManagedInstanceGroup.Type {
	case gcp.Zone:
		l[`compute.googleapis.com/instance_group_manager/zone`] = r.ManagedInstanceGroup.Location
	case gcp.Region:
		l[`compute.googleapis.com/instance_group_manager/region`] = r.ManagedInstanceGroup.Location
	}
	return l
}

type GCEResourceBuilderInterface interface {
	GetResource() (Resource, error)
}

type GCEResourceBuilder struct {
	provider gceDataProvider
}

// Return a resource instance with all the attributes
// based on the single and nested attributes spec
func (gd *GCEResourceBuilder) GetResource() (Resource, error) {
	singleAttributes := map[gceAttribute]string{}
	for attrName, attrGetter := range singleAttributeSpec {
		attr, err := attrGetter(gd.provider)
		if err != nil {
			return nil, err
		}
		singleAttributes[attrName] = attr
	}
	multiAttributes := map[gceAttribute][]string{}
	for attrName, attrGetter := range multiAttributeSpec {
		attr, err := attrGetter(gd.provider)
		if err != nil {
			return nil, err
		}
		multiAttributes[attrName] = attr
	}
	nestedAttributes := map[gceAttribute]map[string]string{}
	for attrName, attrGetter := range nestedAttributeSpec {
		attr, err := attrGetter(gd.provider)
		if err != nil {
			return nil, err
		}
		nestedAttributes[attrName] = attr
	}

	mig, err := gd.provider.getMIG()
	if err != nil {
		return nil, err
	}

	res := GCEResource{
		Project:              singleAttributes[project],
		Zone:                 singleAttributes[zone],
		Network:              singleAttributes[network],
		Subnetwork:           singleAttributes[subnetwork],
		PublicIP:             singleAttributes[publicIP],
		PrivateIP:            singleAttributes[privateIP],
		InstanceID:           singleAttributes[instanceID],
		InstanceName:         singleAttributes[instanceName],
		Tags:                 singleAttributes[tags],
		MachineType:          singleAttributes[machineType],
		DefaultScopes:        multiAttributes[defaultScopes],
		Metadata:             nestedAttributes[metadata],
		Label:                nestedAttributes[label],
		InterfaceIPv4:        nestedAttributes[interfaceIPv4],
		ManagedInstanceGroup: mig,
	}
	return res, nil
}
