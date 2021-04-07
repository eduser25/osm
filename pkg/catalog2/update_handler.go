package catalog2

import (
	"reflect"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set"
	a "github.com/openservicemesh/osm/pkg/announcements"
	"github.com/openservicemesh/osm/pkg/kubernetes/events"
	"github.com/openservicemesh/osm/pkg/logger"
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

func triggerCatalogEnvoyUpdate(proxies mapset.Set, allProxies bool) {
	if allProxies {
		events.GetPubSubInstance().Publish(events.PubSubMessage{
			AnnouncementType: a.ProxyBroadcast,
		})
	} else {
		// TODO: Update the set of proxies only instead
	}
}

func (c *Catalog) updateHandler() {
	subChannel := events.GetPubSubInstance().Subscribe(
		a.EndpointAdded, a.EndpointDeleted, a.EndpointUpdated, // endpoint
		a.NamespaceAdded, a.NamespaceDeleted, a.NamespaceUpdated, // namespace
		a.PodAdded, a.PodDeleted, a.PodUpdated, // pod
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
	proxyUpdateList := mapset.NewSet()

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
			proxiesToUpdate, updateAll := c.handleMessage(&psubMessage)

			proxyUpdateList = proxyUpdateList.Union(proxiesToUpdate)
			allProxyUpdate = allProxyUpdate || updateAll

			if proxiesToUpdate.Cardinality() > 0 || updateAll {
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
			triggerCatalogEnvoyUpdate(proxyUpdateList, allProxyUpdate)

			// broadcast done, reset timer channels
			broadcastScheduled = false
			chanMovingDeadline = make(<-chan time.Time)
			chanMaxDeadline = make(<-chan time.Time)
			allProxyUpdate = false
			proxyUpdateList = mapset.NewSet()

		case <-chanMaxDeadline:
			triggerCatalogEnvoyUpdate(proxyUpdateList, allProxyUpdate)

			// broadcast done, reset timer channels
			broadcastScheduled = false
			chanMovingDeadline = make(<-chan time.Time)
			chanMaxDeadline = make(<-chan time.Time)
			allProxyUpdate = false
			proxyUpdateList = mapset.NewSet()
		}
	}
}

// Update data model here
func (c *Catalog) handleMessage(m *events.PubSubMessage) (mapset.Set, bool) {
	switch m.AnnouncementType {
	default:
		// By default, assume we should trigger a global update
		return mapset.NewSet(), true
	}
}
