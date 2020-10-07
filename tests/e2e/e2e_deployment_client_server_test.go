package e2e

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("N Clients deployments -> 1 Server deployment test", func() {
	Context("DeploymentsClientServer", func() {
		destApp := "server"
		sourceAppBaseName := "client"
		var sourceNamespaces []string = []string{}

		// Total (numberOfClientApps x replicaSetPerApp) pods
		numberOfClientApps := 5
		replicaSetPerApp := 10

		// Used accross the test to wait for concurrent steps to finish
		var wg sync.WaitGroup

		for i := 0; i < numberOfClientApps; i++ {
			sourceNamespaces = append(sourceNamespaces, fmt.Sprintf("%s%d", sourceAppBaseName, i))
		}

		It("Tests HTTP traffic for from multiple client deployments to a server deployment", func() {
			// for cleanup only
			td.cleanupNamespaces["server"] = true
			for _, srcClient := range sourceNamespaces {
				td.cleanupNamespaces[srcClient] = true
			}

			// Install OSM
			Expect(td.InstallOSM(td.GetOSMInstallOpts())).To(Succeed())
			Expect(td.WaitForPodsRunningReady(td.osmMeshName, 60*time.Second, 1)).To(Succeed())

			// Server NS
			Expect(td.CreateNs(destApp, nil)).To(Succeed())
			Expect(td.AddNsToMesh(true, destApp)).To(Succeed())

			// Client Applications
			for _, srcClient := range sourceNamespaces {
				Expect(td.CreateNs(srcClient, nil)).To(Succeed())
				Expect(td.AddNsToMesh(true, srcClient)).To(Succeed())
			}

			// Get simple pod definitions for the HTTP server
			svcAccDef, deploymentDef, svcDef := td.SimpleDeploymentApp(
				SimpleDeploymentAppDef{
					name:         "server",
					namespace:    destApp,
					replicaCount: int32(replicaSetPerApp),
					image:        "kennethreitz/httpbin",
					ports:        []int{80},
				})

			_, err := td.CreateServiceAccount(destApp, &svcAccDef)
			Expect(err).NotTo(HaveOccurred())
			_, err = td.CreateDeployment(destApp, deploymentDef)
			Expect(err).NotTo(HaveOccurred())
			_, err = td.CreateService(destApp, svcDef)
			Expect(err).NotTo(HaveOccurred())

			wg.Add(1)
			go func(wg *sync.WaitGroup, srcClient string) {
				defer wg.Done()
				Expect(td.WaitForPodsRunningReady(destApp, 200*time.Second, replicaSetPerApp)).To(Succeed())
			}(&wg, destApp)

			for _, srcClient := range sourceNamespaces {
				svcAccDef, deploymentDef, svcDef = td.SimpleDeploymentApp(
					SimpleDeploymentAppDef{
						name:         srcClient,
						namespace:    srcClient,
						replicaCount: int32(replicaSetPerApp),
						command:      []string{"/bin/bash", "-c", "--"},
						args:         []string{"while true; do sleep 30; done;"},
						image:        "songrgg/alpine-debug",
						ports:        []int{80}, // Can't deploy services with empty/no ports
					})
				_, err = td.CreateServiceAccount(srcClient, &svcAccDef)
				Expect(err).NotTo(HaveOccurred())
				_, err = td.CreateDeployment(srcClient, deploymentDef)
				Expect(err).NotTo(HaveOccurred())
				_, err = td.CreateService(srcClient, svcDef)
				Expect(err).NotTo(HaveOccurred())

				wg.Add(1)
				go func(wg *sync.WaitGroup, srcClient string) {
					defer wg.Done()
					Expect(td.WaitForPodsRunningReady(srcClient, 200*time.Second, replicaSetPerApp)).To(Succeed())
				}(&wg, srcClient)
			}
			wg.Wait()

			// Create traffic rules
			// Deploy allow rules (10x)clients -> server
			for _, srcClient := range sourceNamespaces {
				httpRG, trafficTarget := td.CreateSimpleAllowPolicy(
					SimpleAllowPolicy{
						RouteGroupName:    srcClient,
						TrafficTargetName: srcClient,

						SourceNamespace:      srcClient,
						SourceSVCAccountName: srcClient,

						DestinationNamespace:      destApp,
						DestinationSvcAccountName: destApp,
					})

				// Configs have to be put into a monitored NS, and osm-system can't be by cli
				_, err = td.CreateHTTPRouteGroup(srcClient, httpRG)
				Expect(err).NotTo(HaveOccurred())
				_, err = td.CreateTrafficTarget(srcClient, trafficTarget)
				Expect(err).NotTo(HaveOccurred())
			}

			// Create Multiple HTTP request structure
			requests := HTTPMultipleRequest{
				Sources: []HTTPRequestDef{},
			}
			for _, ns := range sourceNamespaces {
				pods, err := td.client.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
				Expect(err).To(BeNil())

				for _, pod := range pods.Items {
					requests.Sources = append(requests.Sources, HTTPRequestDef{
						SourceNs:        ns,
						SourcePod:       pod.Name,
						SourceContainer: ns, // container_name == NS for this test

						Destination: fmt.Sprintf("%s.%s", destApp, destApp),

						HTTPUrl: "/",
						Port:    80,
					})
				}
			}

			var results HTTPMultipleResults
			success := WaitForRepeatedSuccess(func() bool {
				var overallSuccess bool = true
				// Get results
				results = td.MultipleHTTPRequest(&requests)
				// Print
				td.PrettyPrintHTTPResults(&results)

				// Verify results
				for _, ns := range results {
					for _, podResult := range ns {
						if podResult.Err != nil || podResult.StatusCode != 200 {
							overallSuccess = false
						}
					}
				}
				return overallSuccess
			}, 5, 150*time.Second)

			Expect(success).To(BeTrue())
		})
	})
})
