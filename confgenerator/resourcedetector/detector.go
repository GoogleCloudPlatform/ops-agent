package resourcedetector

import (
	gcp_metadata "cloud.google.com/go/compute/metadata"
)

// An implementation of the Resource interface will have fields represent
// available attributes about the current monitoring resource.
type Resource interface {
	GetType() string
}

// Get a resource instance for the current environment;
// In order to access the attributes of a specific type of resource,
// needs to cast the returned Resource instance to its underlying type:
// actual, ok := resource.(GCEResource)
func GetResource() (Resource, error) {
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

func (UnrecognizedPlatformResource) GetType() string {
	return "unrecognized platform"
}

func GetUnrecognizedPlatformResource() (Resource, error) {
	return UnrecognizedPlatformResource{}, nil
}
