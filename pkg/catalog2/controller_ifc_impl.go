package catalog2

import (
	"github.com/openservicemesh/osm/pkg/service"
	corev1 "k8s.io/api/core/v1"
)

// ListServices lists the available services
func (c *Catalog) ListServices() []*corev1.Service {
	return c.OriginalKubeController.ListServices()
}

// GetService Returns a corev1 Service representation if the MeshService exists in cache, otherwise nil
func (c *Catalog) GetService(svc service.MeshService) *corev1.Service {
	return c.OriginalKubeController.GetService(svc)
}

// IsMonitoredNamespace returns whether a namespace with the given name is being monitored
// by the mesh
func (c *Catalog) IsMonitoredNamespace(s string) bool {
	_, ok := c.dataModel.namespaces[s]
	return ok
}

// ListMonitoredNamespaces returns the namespaces monitored by the mesh
func (c *Catalog) ListMonitoredNamespaces() ([]string, error) {
	return c.OriginalKubeController.ListMonitoredNamespaces()
}

// GetNamespace returns k8s namespace present in cache
func (c *Catalog) GetNamespace(ns string) *corev1.Namespace {
	return c.OriginalKubeController.GetNamespace(ns)
}

// ListPods returns a list of pods part of the mesh
func (c *Catalog) ListPods() []*corev1.Pod {
	return c.OriginalKubeController.ListPods()
}

// ListServiceAccountsForService lists ServiceAccounts associated with the given service
func (c *Catalog) ListServiceAccountsForService(svc service.MeshService) ([]service.K8sServiceAccount, error) {
	return c.OriginalKubeController.ListServiceAccountsForService(svc)
}

// GetEndpoints returns the endpoints for a given service, if found
func (c *Catalog) GetEndpoints(svc service.MeshService) (*corev1.Endpoints, error) {
	return c.OriginalKubeController.GetEndpoints(svc)
}
