package catalog2

import (
	mapset "github.com/deckarep/golang-set"
	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
)

type NStoNSTrafficMapper map[string]map[string]map[string]TrafficTarget

func (c *Catalog) addMapping(mapper NStoNSTrafficMapper,
	firstKey string,
	secondKey string,
	trafTarget *smiAccess.TrafficTarget) {

	firstLookup, ok := mapper[firstKey]
	if !ok {
		mapper[firstKey] = make(map[string]map[string]TrafficTarget)
		firstLookup = mapper[firstKey]
	}

	secondLookup, ok := firstLookup[secondKey]
	if !ok {
		firstLookup[secondKey] = make(map[string]TrafficTarget)
		secondLookup = firstLookup[secondKey]
	}

	secondLookup[trafTarget.Namespace+"/"+trafTarget.Name] = TrafficTarget{
		trafficTarget: trafTarget,
	}
}

// This function retuurns which namespaces are Traffic-related to the namespace passed by input
func (c *Catalog) getTrafficRelatedNamespaces(ns string) mapset.Set {
	set := mapset.NewSet()

	// Get namespaces this namespace can be talked from
	fromNss, ok := c.dataModel.TrafficTargetFrom[ns]
	if ok {
		for k := range fromNss {
			set.Add(k)
		}
	}

	toNss, ok := c.dataModel.TrafficTargetTo[ns]
	if ok {
		for k := range toNss {
			set.Add(k)
		}
	}

	return set
}
