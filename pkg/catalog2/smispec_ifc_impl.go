package catalog2

import (
	target "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha4"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"

	backpressure "github.com/openservicemesh/osm/experimental/pkg/apis/policy/v1alpha1"
	"github.com/openservicemesh/osm/pkg/service"
)

// ListTrafficSplits lists SMI TrafficSplit resources
func (c *Catalog) ListTrafficSplits() []*split.TrafficSplit {
	return (*c.OriginalSmi).ListTrafficSplits()
}

// ListTrafficSplitServices lists WeightedServices for the services specified in TrafficSplit SMI resources
func (c *Catalog) ListTrafficSplitServices() []service.WeightedService {
	return (*c.OriginalSmi).ListTrafficSplitServices()
}

// ListServiceAccounts lists ServiceAccount resources specified in SMI TrafficTarget resources
func (c *Catalog) ListServiceAccounts() []service.K8sServiceAccount {
	return (*c.OriginalSmi).ListServiceAccounts()
}

// ListHTTPTrafficSpecs lists SMI HTTPRouteGroup resources
func (c *Catalog) ListHTTPTrafficSpecs() []*spec.HTTPRouteGroup {
	return (*c.OriginalSmi).ListHTTPTrafficSpecs()
}

// ListTCPTrafficSpecs lists SMI TCPRoute resources
func (c *Catalog) ListTCPTrafficSpecs() []*spec.TCPRoute {
	return (*c.OriginalSmi).ListTCPTrafficSpecs()
}

// GetTCPRoute returns an SMI TCPRoute resource given its name of the form <namespace>/<name>
func (c *Catalog) GetTCPRoute(route string) *spec.TCPRoute {
	return (*c.OriginalSmi).GetTCPRoute(route)
}

// ListTrafficTargets lists SMI TrafficTarget resources
func (c *Catalog) ListTrafficTargets() []*target.TrafficTarget {
	return (*c.OriginalSmi).ListTrafficTargets()
}

// GetBackpressurePolicy fetches the Backpressure policy for the MeshService
func (c *Catalog) GetBackpressurePolicy(svc service.MeshService) *backpressure.Backpressure {
	return (*c.OriginalSmi).GetBackpressurePolicy(svc)
}
