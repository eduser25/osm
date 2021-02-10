package catalog2

import (
	target "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha4"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"

	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/openservicemesh/osm/pkg/endpoint"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/openservicemesh/osm/pkg/smi"
	"github.com/openservicemesh/osm/pkg/trafficpolicy"
)

// GetSMISpec returns the SMI spec
func (c *Catalog) GetSMISpec() smi.MeshSpec {
	return c // Returning self, as self implements SMISpec
}

// ListTrafficPolicies returns all the traffic policies for a given service that Envoy proxy should be aware of.
func (c *Catalog) ListTrafficPolicies(svc service.MeshService) ([]trafficpolicy.TrafficTarget, error) {
	return c.OriginalCatalog.ListTrafficPolicies(svc)
}

// ListTrafficPoliciesForServiceAccount returns all inbound and outbound traffic policies related to the given service account
func (c *Catalog) ListTrafficPoliciesForServiceAccount(svcAc service.K8sServiceAccount) ([]*trafficpolicy.InboundTrafficPolicy, []*trafficpolicy.OutboundTrafficPolicy, error) {
	return c.OriginalCatalog.ListTrafficPoliciesForServiceAccount(svcAc)
}

// ListAllowedInboundServices lists the inbound services allowed to connect to the given service.
func (c *Catalog) ListAllowedInboundServices(svc service.MeshService) ([]service.MeshService, error) {
	return c.OriginalCatalog.ListAllowedInboundServices(svc)
}

// ListAllowedOutboundServicesForIdentity list the services the given service account is allowed to initiate outbound connections to
func (c *Catalog) ListAllowedOutboundServicesForIdentity(svcAc service.K8sServiceAccount) []service.MeshService {
	return c.OriginalCatalog.ListAllowedOutboundServicesForIdentity(svcAc)
}

// ListAllowedInboundServiceAccounts lists the downstream service accounts that can connect to the given service account
func (c *Catalog) ListAllowedInboundServiceAccounts(svc service.K8sServiceAccount) ([]service.K8sServiceAccount, error) {
	return c.OriginalCatalog.ListAllowedInboundServiceAccounts(svc)
}

// ListAllowedOutboundServiceAccounts lists the upstream service accounts the given service account can connect to
func (c *Catalog) ListAllowedOutboundServiceAccounts(svcAc service.K8sServiceAccount) ([]service.K8sServiceAccount, error) {
	return c.OriginalCatalog.ListAllowedOutboundServiceAccounts(svcAc)
}

// ListSMIPolicies lists SMI policies.
func (c *Catalog) ListSMIPolicies() ([]*split.TrafficSplit, []service.WeightedService, []service.K8sServiceAccount, []*spec.HTTPRouteGroup, []*target.TrafficTarget) {
	return c.OriginalCatalog.ListSMIPolicies()
}

// ListEndpointsForService returns the list of individual instance endpoint backing a service
func (c *Catalog) ListEndpointsForService(svc service.MeshService) ([]endpoint.Endpoint, error) {
	return c.OriginalCatalog.ListEndpointsForService(svc)
}

// GetResolvableServiceEndpoints returns the resolvable set of endpoint over which a service is accessible using its FQDN.
// These are the endpoint destinations we'd expect client applications sends the traffic towards to, when attemtpting to
// reach a specific service.
// If no LB/virtual IPs are assigned to the service, GetResolvableServiceEndpoints will return ListEndpointsForService
func (c *Catalog) GetResolvableServiceEndpoints(svc service.MeshService) ([]endpoint.Endpoint, error) {
	return c.OriginalCatalog.GetResolvableServiceEndpoints(svc)
}

// ExpectProxy catalogs the fact that a certificate was issued for an Envoy proxy and this is expected to connect to XDS.
func (c *Catalog) ExpectProxy(cm certificate.CommonName) {
	c.OriginalCatalog.ExpectProxy(cm)
}

// GetServicesFromEnvoyCertificate returns a list of services the given Envoy is a member of based on the certificate provided, which is a cert issued to an Envoy for XDS communication (not Envoy-to-Envoy).
func (c *Catalog) GetServicesFromEnvoyCertificate(cm certificate.CommonName) ([]service.MeshService, error) {
	return c.OriginalCatalog.GetServicesFromEnvoyCertificate(cm)
}

// RegisterProxy registers a newly connected proxy with the service mesh catalog.
func (c *Catalog) RegisterProxy(p *envoy.Proxy) {
	c.OriginalCatalog.RegisterProxy(p)
}

// UnregisterProxy unregisters an existing proxy from the service mesh catalog
func (c *Catalog) UnregisterProxy(p *envoy.Proxy) {
	c.OriginalCatalog.UnregisterProxy(p)
}

// GetServicesForServiceAccount returns a list of services corresponding to a service account
func (c *Catalog) GetServicesForServiceAccount(svcAc service.K8sServiceAccount) ([]service.MeshService, error) {
	return c.OriginalCatalog.GetServicesForServiceAccount(svcAc)
}

// GetResolvableHostnamesForUpstreamService returns the hostnames over which an upstream service is accessible from a downstream service
func (c *Catalog) GetResolvableHostnamesForUpstreamService(downstream service.MeshService, upstream service.MeshService) ([]string, error) {
	return c.OriginalCatalog.GetResolvableHostnamesForUpstreamService(downstream, upstream)
}

//GetWeightedClusterForService returns the weighted cluster for a service
func (c *Catalog) GetWeightedClusterForService(service service.MeshService) (service.WeightedCluster, error) {
	return c.OriginalCatalog.GetWeightedClusterForService(service)
}

// GetIngressRoutesPerHost returns the HTTP route matches per host associated with an ingress service
func (c *Catalog) GetIngressRoutesPerHost(svc service.MeshService) (map[string][]trafficpolicy.HTTPRouteMatch, error) {
	return c.OriginalCatalog.GetIngressRoutesPerHost(svc)
}

// GetPortToProtocolMappingForService returns a mapping of the service's ports to their corresponding application protocol
func (c *Catalog) GetPortToProtocolMappingForService(svc service.MeshService) (map[uint32]string, error) {
	return c.OriginalCatalog.GetPortToProtocolMappingForService(svc)
}

// GetTargetPortToProtocolMappingForService returns a mapping of the service's ports to their corresponding application protocol.
// The ports returned are the actual ports on which the application exposes the service derived from the service's endpoints,
// ie. 'spec.ports[].targetPort' instead of 'spec.ports[].port' for a Kubernetes service.
// The function ensures the port:protocol mapping is the same across different endpoint providers for the service, and returns
// an error otherwise.
func (c *Catalog) GetTargetPortToProtocolMappingForService(svc service.MeshService) (map[uint32]string, error) {
	return c.OriginalCatalog.GetTargetPortToProtocolMappingForService(svc)
}

// ListInboundTrafficTargetsWithRoutes lists the inbound traffic targets with routes for given service account
func (c *Catalog) ListInboundTrafficTargetsWithRoutes(svc service.K8sServiceAccount) ([]trafficpolicy.TrafficTargetWithRoutes, error) {
	return c.OriginalCatalog.ListInboundTrafficTargetsWithRoutes(svc)
}
