package e2e

import (
	"context"

	"github.com/pkg/errors"
	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	smiSpecs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	smiTrafficAccessClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	smiTrafficSpecClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SmiClients Stores various SMI clients
type SmiClients struct {
	SpecClient   *smiTrafficSpecClient.Clientset
	AccessClient *smiTrafficAccessClient.Clientset
}

// InitSMIClients is called to initialize SMI clients
func (td *OsmTestData) InitSMIClients() error {
	td.SmiClients = &SmiClients{}
	var err error

	td.SmiClients.SpecClient, err = smiTrafficSpecClient.NewForConfig(td.RestConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create traffic spec client")
	}

	td.SmiClients.AccessClient, err = smiTrafficAccessClient.NewForConfig(td.RestConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create traffic acces client")
	}
	return nil
}

// CreateHTTPRouteGroup Creates an SMI Route Group
func (td *OsmTestData) CreateHTTPRouteGroup(ns string, rg smiSpecs.HTTPRouteGroup) (*smiSpecs.HTTPRouteGroup, error) {
	hrg, err := td.SmiClients.SpecClient.SpecsV1alpha3().HTTPRouteGroups(ns).Create(context.Background(), &rg, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTPRouteGroup")
	}
	return hrg, nil
}

// CreateTrafficTarget Creates an SMI TrafficTarget
func (td *OsmTestData) CreateTrafficTarget(ns string, tar smiAccess.TrafficTarget) (*smiAccess.TrafficTarget, error) {
	tt, err := td.SmiClients.AccessClient.AccessV1alpha2().TrafficTargets(ns).Create(context.Background(), &tar, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create TrafficTarget")
	}
	return tt, nil
}

// SimpleAllowPolicy is a simplified struct to later get basic SMI allow policy
type SimpleAllowPolicy struct {
	RouteGroupName string

	TrafficTargetName string

	SourceSVCAccountName string
	SourceNamespace      string

	DestinationSvcAccountName string
	DestinationNamespace      string
}

// CreateSimpleAllowPolicy returns basic allow policy from source to destination, on a HTTP all-wildcard fashion
func (td *OsmTestData) CreateSimpleAllowPolicy(def SimpleAllowPolicy) (smiSpecs.HTTPRouteGroup, smiAccess.TrafficTarget) {
	routeGroup := smiSpecs.HTTPRouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: def.RouteGroupName,
		},
		Spec: smiSpecs.HTTPRouteGroupSpec{
			Matches: []smiSpecs.HTTPMatch{
				{
					Name:      "all",
					PathRegex: ".*",
					Methods:   []string{"*"},
				},
			},
		},
	}

	trafficTarget := smiAccess.TrafficTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: def.TrafficTargetName,
		},
		Spec: smiAccess.TrafficTargetSpec{
			Sources: []smiAccess.IdentityBindingSubject{
				{
					Kind:      "ServiceAccount",
					Name:      def.SourceSVCAccountName,
					Namespace: def.SourceNamespace,
				},
			},
			Destination: smiAccess.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      def.DestinationSvcAccountName,
				Namespace: def.DestinationNamespace,
			},
			Rules: []smiAccess.TrafficTargetRule{
				{
					Kind: "HTTPRouteGroup",
					Name: def.RouteGroupName,
					Matches: []string{
						"all",
					},
				},
			},
		},
	}

	return routeGroup, trafficTarget
}
