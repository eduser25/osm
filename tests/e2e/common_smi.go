package e2e

import (
	"context"

	smiSpecs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	smiSpecClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/typed/specs/v1alpha3"

	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	smiAccessClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned/typed/access/v1alpha2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// SmiClients Stores various SMI clients
type SmiClients struct {
	SpecClient   *smiSpecClient.SpecsV1alpha3Client
	AccessClient *smiAccessClient.AccessV1alpha2Client
}

// InitSMIClients is called to initialize SMI clients
func (td *OsmTestData) InitSMIClients() {
	td.SmiClients = &SmiClients{}

	// Confirmed you need a different RESTconfig per client
	// https://github.com/kubernetes/apiextensions-apiserver/issues/32
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	kubeConfig, err := clientConfig.ClientConfig()
	if err != nil {
		td.T.Fatalf("Failed to get Rest config")
	}

	td.SmiClients.SpecClient, err = smiSpecClient.NewForConfig(kubeConfig)
	if err != nil {
		td.T.Fatalf("Failed to get SmiSpecClient for SMI: %v", err)
	}

	td.SmiClients.AccessClient, err = smiAccessClient.NewForConfig(kubeConfig)
	if err != nil {
		td.T.Fatalf("Failed to get AccessClient for SMI: %v", err)
	}
}

// CreateHTTPRouteGroup Creates an SMI Route Group
func (td *OsmTestData) CreateHTTPRouteGroup(rg smiSpecs.HTTPRouteGroup) {
	_, err := td.SmiClients.SpecClient.HTTPRouteGroups(td.osmMeshName).Create(context.Background(), &rg, metav1.CreateOptions{})
	if err != nil {
		td.T.Fatalf("Error creating HTTP Route Group: %v", err)
	}
}

// CreateTrafficTarget Creates an SMI TrafficTarget
func (td *OsmTestData) CreateTrafficTarget(tar smiAccess.TrafficTarget) {
	_, err := td.SmiClients.AccessClient.TrafficTargets(td.osmMeshName).Create(context.Background(), &tar, metav1.CreateOptions{})
	if err != nil {
		td.T.Fatalf("Error creating traffic Target: %v", err)
	}
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
