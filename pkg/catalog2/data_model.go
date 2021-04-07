package catalog2

import (
	"github.com/openservicemesh/osm/pkg/envoy"
	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
	smiSpecs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha4"
	smiSplit "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	v1 "k8s.io/api/core/v1"
)

// DataModel is used to keep relations between objects
type DataModel struct {
	namespaces    map[string]Namespace
	pod           map[string]map[string]Pod
	service       map[string]map[string]Service
	trafficSplits map[string]map[string]TrafficSplit
	routeGroups   map[string]map[string]RouteGroup
	endpoints     map[string]map[string]Endpoints
	proxies       map[string]map[string]*envoy.Proxy

	// These data structures below keep track the relations between namespaces based on TrafficTargets
	// First one can tell which namespaces can send to <the namespace being looked up> (have at least 1 TrafficTarget to it)
	// Second one can tell which namespaces <the namespace being looked up> can send to (have at least 1 TrafficTarget to)
	// These relations are kept up to date with the update handler, and are used to derive a subset of envoys to update
	// when an update is received.
	// This is for POC only.
	TrafficTargetFrom NStoNSTrafficMapper
	TrafficTargetTo   NStoNSTrafficMapper
}

// NewDataModel returns a new DataModel instance
func NewDataModel() *DataModel {
	return &DataModel{
		namespaces:    make(map[string]Namespace),
		pod:           make(map[string]map[string]Pod),
		service:       make(map[string]map[string]Service),
		trafficSplits: make(map[string]map[string]TrafficSplit),
		routeGroups:   make(map[string]map[string]RouteGroup),
		endpoints:     make(map[string]map[string]Endpoints),
		proxies:       make(map[string]map[string]*envoy.Proxy),

		// Namespace[string] has traffic targets from Namespace[string]
		TrafficTargetFrom: make(map[string]map[string]map[string]TrafficTarget),

		// Namespace[string] has traffic targets to Namespace[string]
		TrafficTargetTo: make(map[string]map[string]map[string]TrafficTarget),
	}
}

// Namespace wraps a K8s Namespace object pointer.
// Will allow for graphing interfaces later on.
type Namespace struct {
	namespace *v1.Namespace
}

type Pod struct {
	pod *v1.Pod
}

type Service struct {
	service *v1.Service
}

type TrafficTarget struct {
	trafficTarget *smiAccess.TrafficTarget
}

type TrafficSplit struct {
	trafficSplit *smiSplit.TrafficSplit
}

type RouteGroup struct {
	routeGroup *smiSpecs.HTTPRouteGroup
}

type Endpoints struct {
	endpoints *v1.Endpoints
}
