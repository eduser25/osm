// Package ads implements Envoy's Aggregated Discovery Service (ADS).
package ads

import (
	"sync"
	"time"

	xds_discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/openservicemesh/osm/pkg/catalog"
	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/openservicemesh/osm/pkg/configurator"
	"github.com/openservicemesh/osm/pkg/envoy"
	k8s "github.com/openservicemesh/osm/pkg/kubernetes"
	"github.com/openservicemesh/osm/pkg/logger"
	"github.com/openservicemesh/osm/pkg/workerpool"
)

var (
	log = logger.New("envoy/ads")
)

// Server implements the Envoy xDS Aggregate Discovery Services
type Server struct {
	catalog        catalog.MeshCataloger
	xdsHandlers    map[envoy.TypeURI]func(catalog.MeshCataloger, *envoy.Proxy, *xds_discovery.DiscoveryRequest, configurator.Configurator, certificate.Manager) ([]types.Resource, error)
	xdsLog         map[certificate.CommonName]map[envoy.TypeURI][]time.Time
	xdsMapLogMutex sync.Mutex
	osmNamespace   string
	cfg            configurator.Configurator
	certManager    certificate.Manager
	ready          bool
	wp             *workerpool.WorkerPool
	kubeController k8s.Controller
	ch             cachev3.SnapshotCache
	srv            serverv3.Server
	mtx            sync.Mutex
	configVersion  map[string]uint64
}
