package resourcedetector

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
	getLabels() (map[string]string, error)
	getMetadata() (map[string]string, error)
	getInterfaceIPv4s() (map[string]string, error)
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

// List of nested attributes
var nestedAttributeSpec = map[gceAttribute]func(gceDataProvider) (map[string]string, error){
	metadata:      gceDataProvider.getMetadata,
	interfaceIPv4: gceDataProvider.getInterfaceIPv4s,
	label:         gceDataProvider.getLabels,
}

// GCEResource implements the Resource interface and provide attributes of the VM when on GCE
type GCEResource struct {
	Project       string
	Zone          string
	Network       string
	Subnetwork    string
	PublicIP      string
	PrivateIP     string
	InstanceID    string
	InstanceName  string
	Tags          string
	MachineType   string
	Metadata      map[string]string
	Label         map[string]string
	InterfaceIPv4 map[string]string
}

func (GCEResource) GetType() string {
	return "gce"
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
	nestedAttributes := map[gceAttribute]map[string]string{}
	for attrName, attrGetter := range nestedAttributeSpec {
		attr, err := attrGetter(gd.provider)
		if err != nil {
			return nil, err
		}
		nestedAttributes[attrName] = attr
	}

	res := GCEResource{
		Project:       singleAttributes[project],
		Zone:          singleAttributes[zone],
		Network:       singleAttributes[network],
		Subnetwork:    singleAttributes[subnetwork],
		PublicIP:      singleAttributes[publicIP],
		PrivateIP:     singleAttributes[privateIP],
		InstanceID:    singleAttributes[instanceID],
		InstanceName:  singleAttributes[instanceName],
		Tags:          singleAttributes[tags],
		MachineType:   singleAttributes[machineType],
		Metadata:      nestedAttributes[metadata],
		Label:         nestedAttributes[label],
		InterfaceIPv4: nestedAttributes[interfaceIPv4],
	}
	return res, nil
}
