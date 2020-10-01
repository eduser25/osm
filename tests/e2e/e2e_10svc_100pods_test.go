package e2e

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/ahmetalpbalkan/go-cursor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Test 10(x10 pods) Clients -> 1(x10 pods) server", func() {
	Context("DeploymentsClientServer", func() {
		destApp := "server"
		sourceAppBaseName := "client"
		var sourceNamespaces []string = []string{} // for this test, (Service name == NS name == container name)
		var podsPerNamespace map[string][]string = map[string][]string{}

		// Total (numberOfClientApps x replicaSetPerApp) pods
		numberOfClientApps := 5
		replicaSetPerApp := 10

		// Used accross the test to wait for concurrent steps to finish
		var wg sync.WaitGroup

		for i := 0; i < numberOfClientApps; i++ {
			sourceNamespaces = append(sourceNamespaces, fmt.Sprintf("%s%d", sourceAppBaseName, i))
		}

		It("Tests HTTP traffic for a simple client-server pod deployment", func() {
			// For Cleanup only
			td.Namespaces["server"] = true
			for _, srcClient := range sourceNamespaces {
				td.Namespaces[srcClient] = true
			}

			// Install OSM
			Expect(td.InstallOSM(td.GetTestInstallOpts())).To(Succeed())
			Expect(td.WaitForPodsRunningReady(td.osmMeshName, 60*time.Second, 1)).To(Succeed())

			// Create Test NS
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

			// Get Deterministic list of pods per source application
			// Walking maps is NOT deterministic in golang, this causes scrambled output later when walking map ranges repeatedly
			for _, ns := range sourceNamespaces {
				// Get pods for every service (assuming every service is enclosed in a namespace, we can get all pods for an NS)
				pods, err := td.Client.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
				Expect(err).To(BeNil())

				for _, pod := range pods.Items {
					_, found := podsPerNamespace[ns]
					if !found {
						podsPerNamespace[ns] = []string{}
					}
					podsPerNamespace[ns] = append(podsPerNamespace[ns], pod.Name)
				}
			}

			// Map[ns][pod] -> HTTPResults
			var results map[string]map[string]*HTTPAsyncResults = make(map[string]map[string]*HTTPAsyncResults)
			// synchronization artifacts. We will reuse the global waitgroup
			var stop bool = false

			// Start HTTP async threads
			for _, ns := range sourceNamespaces {
				for _, pod := range podsPerNamespace[ns] {
					// Create a Result
					_, ok := results[ns]
					if !ok {
						results[ns] = make(map[string]*HTTPAsyncResults)
					}
					results[ns][pod] = NewHTTPAsyncResults()

					go td.AsynchronousHTTPRunner(
						HTTPAsync{
							RequestData: HTTPRequestDef{
								SourceNs:        ns,
								SourcePod:       pod,
								SourceContainer: ns, // container_name == NS for this test

								Destination: fmt.Sprintf("%s.%s", destApp, destApp),

								HTTPUrl: "/",
								Port:    80,
							},
							ResponseDataStore: results[ns][pod],
							StopSignal:        &stop,
							WaitGroup:         &wg,
							SleepTime:         1 * time.Second,
						},
					)
				}
			}

			// Inspect results and wait for success/failure
			success := WaitForRepeatedSuccess(func() bool {
				var overallSuccess bool = true

				for _, ns := range sourceNamespaces {
					podNames := []string{}
					podResultsToPrint := []*HTTPAsyncResults{}

					for _, pod := range podsPerNamespace[ns] {
						podRes := results[ns][pod]

						podResultsToPrint = append(podResultsToPrint, podRes)
						podNames = append(podNames, pod)

						podRes.withLock(func() {
							if podRes.lastResult.Err != nil || podRes.lastResult.StatusCode != 200 {
								overallSuccess = false
							}
						})
					}
					td.PrettyPrintHTTPResults(ns, podNames, podResultsToPrint)
				}
				// Meaningful only on shell
				td.T.Log(cursor.MoveUp(len(sourceNamespaces) + 1))

				return overallSuccess
			}, 5, 150*time.Second)

			// Meaningful only on shell
			td.T.Log(cursor.MoveDown(len(sourceNamespaces) + 1))

			// Stop all async threads
			stop = true
			wg.Wait()

			Expect(success).To(BeTrue())
		})
	})
})
