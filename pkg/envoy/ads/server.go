package ads

import (
	"context"
	"sync"
	"time"

	xds_discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"

	"github.com/openservicemesh/osm/pkg/catalog"
	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/openservicemesh/osm/pkg/configurator"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/envoy/cds"
	"github.com/openservicemesh/osm/pkg/envoy/eds"
	"github.com/openservicemesh/osm/pkg/envoy/lds"
	"github.com/openservicemesh/osm/pkg/envoy/rds"
	"github.com/openservicemesh/osm/pkg/envoy/sds"
	k8s "github.com/openservicemesh/osm/pkg/kubernetes"
	"github.com/openservicemesh/osm/pkg/utils"
	"github.com/openservicemesh/osm/pkg/workerpool"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	logg "github.com/sirupsen/logrus"
)

// ServerType is the type identifier for the ADS server
const ServerType = "ADS"

// NewADSServer creates a new Aggregated Discovery Service server
func NewADSServer(meshCatalog catalog.MeshCataloger, enableDebug bool, osmNamespace string, cfg configurator.Configurator, certManager certificate.Manager, kc k8s.Controller) *Server {
	server := Server{
		catalog: meshCatalog,
		xdsHandlers: map[envoy.TypeURI]func(catalog.MeshCataloger, *envoy.Proxy, *xds_discovery.DiscoveryRequest, configurator.Configurator, certificate.Manager) ([]types.Resource, error){
			envoy.TypeEDS: eds.NewResponse,
			envoy.TypeCDS: cds.NewResponse,
			envoy.TypeRDS: rds.NewResponse,
			envoy.TypeLDS: lds.NewResponse,
			envoy.TypeSDS: sds.NewResponse,
		},
		osmNamespace:   osmNamespace,
		cfg:            cfg,
		certManager:    certManager,
		xdsMapLogMutex: sync.Mutex{},
		xdsLog:         make(map[certificate.CommonName]map[envoy.TypeURI][]time.Time),
		wp:             workerpool.NewWorkerPool(0),
		kubeController: kc,
		mtx:            sync.Mutex{},
		configVersion:  make(map[string]uint64),
	}

	return &server
}

// withXdsLogMutex helper to run code that touches xdsLog map, to protect by mutex
func (s *Server) withXdsLogMutex(f func()) {
	s.xdsMapLogMutex.Lock()
	defer s.xdsMapLogMutex.Unlock()
	f()
}

// Start starts the ADS server
func (s *Server) Start(ctx context.Context, cancel context.CancelFunc, port int, adsCert certificate.Certificater) error {
	logrus := logg.New()
	logrus.SetLevel(logg.DebugLevel)
	s.ch = cachev3.NewSnapshotCache(true, cachev3.IDHash{}, logrus)
	s.srv = serverv3.NewServer(ctx, s.ch, &Callbacks{Debug: true})

	grpcServer, lis, err := utils.NewGrpc(ServerType, port, adsCert.GetCertificateChain(), adsCert.GetPrivateKey(), adsCert.GetIssuingCA())
	if err != nil {
		log.Error().Err(err).Msg("Error starting ADS server")
		return err
	}

	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, s.srv)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, s.srv)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, s.srv)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, s.srv)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, s.srv)
	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, s.srv)
	runtimeservice.RegisterRuntimeDiscoveryServiceServer(grpcServer, s.srv)

	go utils.GrpcServe(ctx, grpcServer, lis, cancel, ServerType, nil)
	go s.broadcastListener()

	s.ready = true

	return nil
}

// DeltaAggregatedResources implements discovery.AggregatedDiscoveryServiceServer
func (s *Server) DeltaAggregatedResources(xds_discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesServer) error {
	panic("NotImplemented")
}
