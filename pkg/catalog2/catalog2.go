package catalog2

import (
	"sync"

	"github.com/openservicemesh/osm/pkg/catalog"
	k8s "github.com/openservicemesh/osm/pkg/kubernetes"
	"github.com/openservicemesh/osm/pkg/smi"
)

// Catalog is the object that holds the internal datamodel and structures where information is related
type Catalog struct {
	OriginalKubeController *k8s.Client
	OriginalCatalog        *catalog.MeshCatalog
	OriginalSmi            *smi.MeshSpec

	dataModel     *DataModel
	dataModelLock sync.RWMutex
}

// WithRlock is used to perform operation on the data model preventing writes from Notifications
func (c *Catalog) WithRlock(f func()) {
	c.dataModelLock.RLock()
	defer c.dataModelLock.RUnlock()
	f()
}

// NewCatalog creates a new service catalog
func NewCatalog() *Catalog {
	catalog := &Catalog{
		dataModel:     NewDataModel(),
		dataModelLock: sync.RWMutex{},
	}

	go catalog.updateHandler()

	return catalog
}
