package ads

import (
	"context"
	"hash/fnv"
	"net"
	"strings"
	"sync"

	mapset "github.com/deckarep/golang-set"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"

	"github.com/openservicemesh/osm/pkg/announcements"
	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/kubernetes/events"
)

// Routine which fullfills listening to proxy broadcasts
func (s *Server) broadcastListener() {
	// Register to Envoy global broadcast updates
	broadcastUpdate := events.GetPubSubInstance().Subscribe(announcements.ProxyBroadcast)
	for {
		<-broadcastUpdate
		s.allPodUpdater()
	}
}

// -- implementation of a "sendResponse" job
// read it as "create new configuration for a given envoy" job that can be queued to a worker
type fullAdsJob struct {
	S     *Server
	Proxy *envoy.Proxy
}

func (adsj *fullAdsJob) Run() {
	adsj.S.sendResponse(mapset.NewSetWith(
		envoy.TypeCDS,
		envoy.TypeEDS,
		envoy.TypeLDS,
		envoy.TypeRDS,
		envoy.TypeSDS), adsj.Proxy, nil, adsj.S.cfg)
}

func (adsj *fullAdsJob) JobName() string {
	return "SendAllResponses"
}

func (adsj *fullAdsJob) Hash() uint64 {
	// make jobs for the same proxy hash to the same worker, implied serialization
	// for the same proxy
	algorithm := fnv.New64a()
	algorithm.Write([]byte(adsj.Proxy.GetCertificateCommonName()))
	return algorithm.Sum64()
}

func (s *Server) allPodUpdater() {
	allpods := s.kubeController.ListPods()

	for _, pod := range allpods {
		proxy, err := GetProxyFromPod(pod)
		if err != nil {
			log.Error().Err(err).Msgf("Could not get proxy from pod %s/%s", pod.Namespace, pod.Name)
			continue
		}

		// Queue update for this proxy/pod
		updatePodJob := fullAdsJob{
			S:     s,
			Proxy: proxy,
		}
		s.wp.AddJob(&updatePodJob)
	}
}

// Infer and create a Proxy data structure from a Pod one.
// Our xDS paths only (functionally) use GetCN to get anything out
// from the proxy. (Serial and PodUUID are only used in logging, appears)
func GetProxyFromPod(pod *v1.Pod) (*envoy.Proxy, error) {
	var uuid string
	var serviceAccount string
	var namespace string

	uuid, uuidFound := pod.Labels[constants.EnvoyUniqueIDLabelName]
	if !uuidFound {
		return nil, errors.Errorf("UUID not found for pod %s/%s, not a mesh pod", pod.Namespace, pod.Name)
	}

	serviceAccount = pod.Spec.ServiceAccountName
	namespace = pod.Namespace

	// construct CN for this pod/proxy
	commonName := strings.Join([]string{uuid, serviceAccount, namespace}, constants.DomainDelimiter)

	return envoy.NewProxy(certificate.CommonName(commonName), "DummySerialNumber", &net.IPAddr{IP: net.IPv4zero}), nil
}

// Implementation of xDS server callbacks below
// These are put in case we want to add any logic to specific parts of the xDS server proto handling implementation,
// they are NOT required to do anything.
// Sample implementation from https://github.com/envoyproxy/go-control-plane/blob/main/docs/cache/Server.md
// It just counts and allows to log in between this functions, nothing more.

type Callbacks struct {
	Signal   chan struct{}
	Debug    bool
	Fetches  int
	Requests int
	mu       sync.Mutex
}

func (cb *Callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log.Debug().Msgf("server callbacks fetches=%d requests=%d\n", cb.Fetches, cb.Requests)
}

func (cb *Callbacks) OnStreamOpen(_ context.Context, id int64, typ string) error {
	if cb.Debug {
		log.Debug().Msgf("stream %d open for %s\n", id, typ)
	}
	return nil
}

func (cb *Callbacks) OnStreamClosed(id int64) {
	if cb.Debug {
		log.Debug().Msgf("stream %d closed\n", id)
	}
}

func (cb *Callbacks) OnStreamRequest(a int64, req *discovery.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.Requests++
	if cb.Signal != nil {
		close(cb.Signal)
		cb.Signal = nil
	}

	log.Warn().Msgf("REQUEST OnStreamRequest NODE: [%s]", req.Node.Id)
	log.Warn().Msgf("REQUEST OnStreamRequest: [%s]", req.TypeUrl)
	log.Warn().Msgf("REQUEST OnStreamRequest: [%v]", req.ResourceNames)

	return nil
}

func (cb *Callbacks) OnStreamResponse(aa int64, req *discovery.DiscoveryRequest, resp *discovery.DiscoveryResponse) {
	log.Warn().Msgf("RESPONSE NODE: [%s]", req.Node.Id)
	log.Warn().Msgf("REQUEST: [%s]", req.TypeUrl)
	log.Warn().Msgf("REQUEST: [%v]", req.ResourceNames)
	log.Warn().Msgf("RESPONSE: num resources: %d", len(resp.Resources))
	//log.Warn().Msgf("RESPONSE: [%v]", resources)

}

func (cb *Callbacks) OnFetchRequest(_ context.Context, req *discovery.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.Fetches++
	if cb.Signal != nil {
		close(cb.Signal)
		cb.Signal = nil
	}
	return nil
}
func (cb *Callbacks) OnFetchResponse(req *discovery.DiscoveryRequest, resp *discovery.DiscoveryResponse) {
	log.Warn().Msgf("FETCH NODE: [%s]", req.Node.Id)
	log.Warn().Msgf("REQUEST: [%s]", req.TypeUrl)
	log.Warn().Msgf("REQUEST: [%v]", req.ResourceNames)
	log.Warn().Msgf("RESPONSE: num resources: %d", len(resp.Resources))
	//log.Warn().Msgf("RESPONSE: [%v]", resources)
}
