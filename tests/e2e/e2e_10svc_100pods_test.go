package e2e

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test 10(x10 pods) Clients -> 1(x10 pods) server", func() {
	Context("DeploymentsClientServer", func() {
		destApp := "server"
		sourceAppBaseName := "client"
		sourceApplications := []string{}

		// Total 100 pods
		numberOfClientApps := 10
		replicaSetPerApp := 10

		for i := 0; i < numberOfClientApps; i++ {
			sourceApplications = append(sourceApplications, fmt.Sprintf("%s%d", sourceAppBaseName, i))
		}

		It("Tests HTTP traffic for a simple client-server pod deployment", func() {
			// Install OSM
			Expect(td.InstallOSM(td.GetTestInstallOpts())).To(Succeed())
			Expect(td.WaitForPodsRunningReady(td.osmMeshName, 60*time.Second, 1)).To(Succeed())

			// Create Test NS
			// Server NS
			Expect(td.CreateNs(destApp, nil)).To(Succeed())
			Expect(td.AddNsToMesh(true, destApp)).To(Succeed())

			// Client Applications
			for _, srcClient := range sourceApplications {
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

			// Expect it to be up and running in it's receiver namespace
			Expect(td.WaitForPodsRunningReady(destApp, 200*time.Second, replicaSetPerApp)).To(Succeed())

			// Jumpstart clients
			var wg sync.WaitGroup
			for _, srcClient := range sourceApplications {
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

			// td.CreateServiceAccount(sourceNs, &svcAccDef)
			// srcPod, _ := td.CreatePod(sourceNs, podDef)
			// td.CreateService(sourceNs, svcDef)

			// // Expect it to be up and running in it's receiver namespace
			// td.WaitForPodsRunningReady(sourceNs, 60*time.Second)

			// // Deploy allow rule client->server
			// httpRG, trafficTarget := td.CreateSimpleAllowPolicy(
			// 	SimpleAllowPolicy{
			// 		RouteGroupName:    "routes",
			// 		TrafficTargetName: "test-target",

			// 		SourceNamespace:      sourceNs,
			// 		SourceSVCAccountName: "client",

			// 		DestinationNamespace:      destNs,
			// 		DestinationSvcAccountName: "server",
			// 	})

			// // Configs have to be put into a monitored NS, and osm-system can't be by cli
			// td.CreateHTTPRouteGroup(sourceNs, httpRG)
			// td.CreateTrafficTarget(sourceNs, trafficTarget)

			// // All ready. Expect client to reach server
			// // Need to get the pod though.
			// if !WaitForRepeatedSuccess(func() bool {
			// 	statusCode, _, err :=
			// 		td.HTTPRequest(HTTPRequestDef{
			// 			SourceNs:        srcPod.Namespace,
			// 			SourcePod:       srcPod.Name,
			// 			SourceContainer: "client", // We can do better

			// 			Destination: fmt.Sprintf("%s.%s", dstPod.Name, dstPod.Namespace),

			// 			HTTPUrl: "/",
			// 			Port:    80,
			// 		})

			// 	if err != nil || statusCode != 200 {
			// 		td.T.Logf("> REST req failed (status: %d) %v", statusCode, err)
			// 		return false
			// 	}
			// 	td.T.Logf("> REST req succeeded: %d", statusCode)
			// 	return true
			// }, 5, 60*time.Second) {
			// 	td.T.Logf("Test Failed")
			// 	td.T.FailNow()
			// } else {
			// 	td.T.Logf("Test Passed")
			// }
		})
	})
})
