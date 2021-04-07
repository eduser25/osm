package catalog2

import (
	"reflect"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set"
	a "github.com/openservicemesh/osm/pkg/announcements"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/kubernetes/events"
	"github.com/openservicemesh/osm/pkg/logger"
	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
	smiSpecs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha4"
	smiSplit "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	v1 "k8s.io/api/core/v1"
)

var (
	log = logger.New("catalog2")
)

const (
	// maxBroadcastDeadlineTime is the max time we will delay a global proxy update
	// if multiple events that would trigger it get coalesced over time.
	maxBroadcastDeadlineTime = 15 * time.Second
	// maxGraceDeadlineTime is the time we will wait for an additinal global proxy update
	// trigger if we just received one.
	maxGraceDeadlineTime = 3 * time.Second
)

// isDeltaUpdate assesses and returns if a pubsub mesasge contains an actual delta in config
func isDeltaUpdate(psubMsg events.PubSubMessage) bool {
	return !(strings.HasSuffix(psubMsg.AnnouncementType.String(), "updated") &&
		reflect.DeepEqual(psubMsg.OldObj, psubMsg.NewObj))
}

func (c *Catalog) triggerCatalogEnvoyUpdate(namespaces mapset.Set, allProxies bool) {
	if allProxies {
		events.GetPubSubInstance().Publish(events.PubSubMessage{
			AnnouncementType: a.ProxyBroadcast,
		})
	} else {
		log.Warn().Msgf("[catalog2] triggering update for namespaces %v", namespaces)
		namespaces.Each(func(ns interface{}) bool {
			nss := ns.(string)
			proxyNs, ok := c.dataModel.proxies[nss]
			if ok {
				log.Warn().Msgf("[catalog2] found proxies ns %s", nss)
				for _, proxObj := range proxyNs {
					log.Warn().Msgf("[catalog2] Issuing proxy update for  %s", proxObj.GetCertificateCommonName().String())
					proxObj.Announcements <- a.Announcement{}
				}
			} else {
				log.Warn().Msgf("[catalog2] NO NAMESPACE PROXIES IN %s", nss)
			}
			return false
		})
	}
}

func (c *Catalog) updateHandler() {
	subChannel := events.GetPubSubInstance().Subscribe(
		a.EndpointAdded, a.EndpointDeleted, a.EndpointUpdated, // endpoint
		a.NamespaceAdded, a.NamespaceDeleted, a.NamespaceUpdated, // namespace
		a.PodAdded, a.PodDeleted, a.PodUpdated, // pod
		a.ProxyAdded, a.ProxyDeleted, a.ProxyUpdated, // proxy
		a.ServiceAdded, a.ServiceDeleted, a.ServiceUpdated, // service
		a.RouteGroupAdded, a.RouteGroupDeleted, a.RouteGroupUpdated, // routegroup
		a.TrafficSplitAdded, a.TrafficSplitDeleted, a.TrafficSplitUpdated, // traffic split
		a.TrafficTargetAdded, a.TrafficTargetDeleted, a.TrafficTargetUpdated, // traffic target
		a.IngressAdded, a.IngressDeleted, a.IngressUpdated, // Ingress
		a.TCPRouteAdded, a.TCPRouteDeleted, a.TCPRouteUpdated, // TCProute
	)

	// State and channels for event-coalescing
	broadcastScheduled := false
	chanMovingDeadline := make(<-chan time.Time)
	chanMaxDeadline := make(<-chan time.Time)
	allProxyUpdate := false
	namespaceUpdateList := mapset.NewSet()

	for {
		select {
		case message := <-subChannel:

			// New message from pubsub
			psubMessage, castOk := message.(events.PubSubMessage)
			if !castOk {
				log.Error().Msgf("Error casting PubSubMessage: %v", psubMessage)
				continue
			}

			// Identify if this is an actual delta, or just resync
			delta := isDeltaUpdate(psubMessage)
			log.Debug().Msgf("[Pubsub] %s - delta: %v", psubMessage.AnnouncementType.String(), delta)
			if !delta {
				// Nothing here
				continue
			}

			// update Object model/identify envoy updates
			namespaces, updateAll := c.handleMessage(&psubMessage)

			namespaceUpdateList = namespaceUpdateList.Union(namespaces)
			allProxyUpdate = allProxyUpdate || updateAll

			if namespaceUpdateList.Cardinality() > 0 || updateAll {
				if !broadcastScheduled {
					broadcastScheduled = true
					chanMaxDeadline = time.After(maxBroadcastDeadlineTime)
					chanMovingDeadline = time.After(maxGraceDeadlineTime)
				} else {
					// If a broadcast is already scheduled, just reset the moving deadline
					chanMovingDeadline = time.After(maxGraceDeadlineTime)
				}
			} else {
				// It was a config delta, but we did not identify any envoys that require an update
				continue
			}

		// A select-fallthrough doesn't exist, we are copying some code here
		case <-chanMovingDeadline:
			log.Warn().Msgf("[catalog2] movingDeadline %v %v", allProxyUpdate, namespaceUpdateList)
			c.triggerCatalogEnvoyUpdate(namespaceUpdateList, allProxyUpdate)

			// broadcast done, reset timer channels
			broadcastScheduled = false
			chanMovingDeadline = make(<-chan time.Time)
			chanMaxDeadline = make(<-chan time.Time)
			allProxyUpdate = false
			namespaceUpdateList = mapset.NewSet()

		case <-chanMaxDeadline:
			log.Warn().Msgf("[catalog2] maxDeadline %v %v", allProxyUpdate, namespaceUpdateList)
			c.triggerCatalogEnvoyUpdate(namespaceUpdateList, allProxyUpdate)

			// broadcast done, reset timer channels
			broadcastScheduled = false
			chanMovingDeadline = make(<-chan time.Time)
			chanMaxDeadline = make(<-chan time.Time)
			allProxyUpdate = false
			namespaceUpdateList = mapset.NewSet()
		}
	}
}

// Update data model here
func (c *Catalog) handleMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	switch m.AnnouncementType {
	case a.NamespaceAdded, a.NamespaceUpdated, a.NamespaceDeleted:
		return c.handleNamespaceMessage(m)
	case a.PodAdded, a.PodDeleted, a.PodUpdated:
		return c.handlePodMessage(m)
	case a.ServiceAdded, a.ServiceDeleted, a.ServiceUpdated:
		return c.handleServiceMessage(m)
	case a.TrafficSplitAdded, a.TrafficSplitDeleted, a.TrafficSplitUpdated:
		return c.handleTrafficSplitMessage(m)
	case a.TrafficTargetAdded, a.TrafficTargetDeleted, a.TrafficTargetUpdated:
		return c.handleTrafficTarget(m)
	case a.RouteGroupAdded, a.RouteGroupDeleted, a.RouteGroupUpdated:
		return c.handleRouteGroupMessage(m)
	case a.ProxyAdded, a.ProxyDeleted, a.ProxyUpdated:
		return c.handleProxyMessage(m)
	default:
		// By default, assume we should trigger a global update
		return mapset.NewSet(), false
	}
}

func (c *Catalog) handleNamespaceMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	if m.AnnouncementType == a.NamespaceDeleted || m.AnnouncementType == a.NamespaceUpdated {
		oldNs := m.OldObj.(*v1.Namespace)
		c.WithWlock(func() {
			delete(c.dataModel.namespaces, oldNs.Name)
		})
	}
	if m.AnnouncementType == a.NamespaceAdded || m.AnnouncementType == a.NamespaceUpdated {
		newNs := m.NewObj.(*v1.Namespace)
		c.WithWlock(func() {
			c.dataModel.namespaces[newNs.Name] = Namespace{
				namespace: newNs,
			}
		})
	}
	return mapset.NewSet(), false
}

func (c *Catalog) handlePodMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	var ns string
	if m.AnnouncementType == a.PodDeleted || m.AnnouncementType == a.PodUpdated {
		oldPod := m.OldObj.(*v1.Pod)
		ns = oldPod.Namespace
		c.WithWlock(func() {
			podNs, ok := c.dataModel.pod[oldPod.Namespace]
			if ok {
				delete(podNs, oldPod.Name)
			}
		})
	}
	if m.AnnouncementType == a.PodAdded || m.AnnouncementType == a.PodUpdated {
		newPod := m.NewObj.(*v1.Pod)
		ns = newPod.Namespace
		c.WithWlock(func() {
			podNs, ok := c.dataModel.pod[newPod.Namespace]
			if !ok {
				c.dataModel.pod[newPod.Namespace] = make(map[string]Pod)
				podNs = c.dataModel.pod[newPod.Namespace]
			}
			podNs[newPod.Name] = Pod{
				pod: newPod,
			}
		})
	}
	return c.getTrafficRelatedNamespaces(ns), false
}

func (c *Catalog) handleServiceMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	var ns string
	if m.AnnouncementType == a.ServiceDeleted || m.AnnouncementType == a.ServiceUpdated {
		oldService := m.OldObj.(*v1.Service)
		ns = oldService.Namespace
		c.WithWlock(func() {
			serviceNs, ok := c.dataModel.service[oldService.Namespace]
			if ok {
				delete(serviceNs, oldService.Name)
			}
		})
	}
	if m.AnnouncementType == a.ServiceAdded || m.AnnouncementType == a.ServiceUpdated {
		newService := m.NewObj.(*v1.Service)
		ns = newService.Namespace
		c.WithWlock(func() {
			serviceNs, ok := c.dataModel.service[newService.Namespace]
			if !ok {
				c.dataModel.service[newService.Namespace] = make(map[string]Service)
				serviceNs = c.dataModel.service[newService.Namespace]
			}
			serviceNs[newService.Name] = Service{
				service: newService,
			}
		})
	}
	return c.getTrafficRelatedNamespaces(ns), false
}

func (c *Catalog) handleTrafficTarget(m *events.PubSubMessage) (mapset.Set, bool) {
	set := mapset.NewSet()
	if m.AnnouncementType == a.TrafficTargetDeleted || m.AnnouncementType == a.TrafficTargetUpdated {
		// TODO (for POC)
	}
	if m.AnnouncementType == a.TrafficTargetAdded || m.AnnouncementType == a.TrafficTargetUpdated {
		newTrafficTarget := m.NewObj.(*smiAccess.TrafficTarget)
		c.WithWlock(func() {
			// Add the destination NS to the To mapper
			dstNamespace := newTrafficTarget.Spec.Destination.Namespace
			set.Add(dstNamespace)

			for _, src := range newTrafficTarget.Spec.Sources {
				srcNamespace := src.Namespace
				set.Add(srcNamespace)

				// Add that dstNamespace has a TrafficTarget from srcNamespace
				c.addMapping(c.dataModel.TrafficTargetFrom,
					dstNamespace,
					srcNamespace,
					newTrafficTarget)

				// Add that srcNamespace has a TrafficTarget to dstNamespace
				c.addMapping(c.dataModel.TrafficTargetTo,
					srcNamespace,
					dstNamespace,
					newTrafficTarget)
			}
		})
	}
	return set, false
}

func (c *Catalog) handleTrafficSplitMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	var ns string
	if m.AnnouncementType == a.TrafficSplitDeleted || m.AnnouncementType == a.TrafficSplitUpdated {
		// TODO (for POC)
	}
	if m.AnnouncementType == a.TrafficSplitAdded || m.AnnouncementType == a.TrafficSplitUpdated {
		newTfSplit := m.NewObj.(*smiSplit.TrafficSplit)
		ns = newTfSplit.Namespace
		c.dataModelLock.Lock()
		tfSplitNs, ok := c.dataModel.trafficSplits[newTfSplit.Namespace]
		if !ok {
			c.dataModel.trafficSplits[newTfSplit.Namespace] = make(map[string]TrafficSplit)
			tfSplitNs = c.dataModel.trafficSplits[newTfSplit.Namespace]
		}
		tfSplitNs[newTfSplit.Name] = TrafficSplit{
			trafficSplit: newTfSplit,
		}

		c.dataModelLock.Unlock()
	}
	return c.getTrafficRelatedNamespaces(ns), false
}

func (c *Catalog) handleRouteGroupMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	var ns string
	if m.AnnouncementType == a.RouteGroupDeleted || m.AnnouncementType == a.RouteGroupUpdated {
		// TODO (for POC)
	}
	if m.AnnouncementType == a.RouteGroupAdded || m.AnnouncementType == a.RouteGroupUpdated {
		rgNew := m.NewObj.(*smiSpecs.HTTPRouteGroup)
		ns = rgNew.Namespace
		c.WithWlock(func() {
			rgNs, ok := c.dataModel.routeGroups[rgNew.Namespace]
			if !ok {
				c.dataModel.routeGroups[rgNew.Namespace] = make(map[string]RouteGroup)
				rgNs = c.dataModel.routeGroups[rgNew.Namespace]
			}
			rgNs[rgNew.Name] = RouteGroup{
				routeGroup: rgNew,
			}
		})
	}
	return c.getTrafficRelatedNamespaces(ns), false
}

func (c *Catalog) handleProxyMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	var ns string
	if m.AnnouncementType == a.ProxyDeleted || m.AnnouncementType == a.ProxyUpdated {
		// TODO (for POC)
	}
	if m.AnnouncementType == a.ProxyAdded || m.AnnouncementType == a.ProxyUpdated {
		newProx := m.NewObj.(*envoy.Proxy)
		svcList, err := c.GetServicesFromEnvoyCertificate(newProx.GetCertificateCommonName())

		if err != nil || len(svcList) == 0 {
			log.Fatal().Msgf("NO Service for proxy %s %v", newProx.GetCertificateCommonName().String(), err)
		}

		proxyServiceName := svcList[0]
		ns = proxyServiceName.Namespace

		c.WithWlock(func() {
			nsProx, ok := c.dataModel.proxies[proxyServiceName.Namespace]
			if !ok {
				c.dataModel.proxies[proxyServiceName.Namespace] = make(map[string]*envoy.Proxy)
				nsProx = c.dataModel.proxies[proxyServiceName.Namespace]
			}

			nsProx[newProx.GetCertificateCommonName().String()] = newProx

		})
	}
	return c.getTrafficRelatedNamespaces(ns), false
}

func (c *Catalog) handleEndpointMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	var ns string
	if m.AnnouncementType == a.EndpointDeleted || m.AnnouncementType == a.EndpointUpdated {
		// TODO (for POC)
	}
	if m.AnnouncementType == a.EndpointAdded || m.AnnouncementType == a.EndpointUpdated {
		endpoints := m.NewObj.(*v1.Endpoints)
		ns = endpoints.Namespace

		c.WithWlock(func() {
			endpNs, ok := c.dataModel.endpoints[endpoints.Namespace]
			if !ok {
				c.dataModel.endpoints[endpoints.Namespace] = make(map[string]Endpoints)
				endpNs = c.dataModel.endpoints[endpoints.Namespace]
			}

			endpNs[endpoints.Name] = Endpoints{
				endpoints: endpoints,
			}
		})
	}
	return c.getTrafficRelatedNamespaces(ns), false
}
