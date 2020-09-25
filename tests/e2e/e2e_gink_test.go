package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
)

// Since parseFlags is global, this is the Ginkgo way to do it. Cant help it.
// https://github.com/onsi/ginkgo/issues/265
var td OsmTestData

func init() {
	registerFlags(&td)
}

var _ = Describe("Test and deploy a simple mesh", func() {
	Context("Test OSM testing APIs", func() {

		sourceNs := "client"
		destNs := "server"
		var ns []string = []string{sourceNs, destNs}

		It("Test testing APIs in a simple e2e test", func() {
			td.InitTestData(GinkgoT())
			// For cleanup only while testing, not needed
			for _, n := range ns {
				td.Namespaces[n] = true
			}

			// Install OSM
			td.InstallOSM(td.GetTestInstallOpts())
			td.WaitForPodsRunningReady(td.osmMeshName, 40*time.Second)

			// Create Test NS
			for _, n := range ns {
				td.CreateNs(n, nil)
			}

			// Add Namespaces to mesh
			td.AddNsToMesh(true, ns...)

			// Get simple pod definitions for the HTTP server
			svcAccDef, podDef, svcDef := td.SimplePodApp(
				SimplePodAppDef{
					name:      "server",
					namespace: destNs,
					image:     "kennethreitz/httpbin",
				})

			td.CreateServiceAccount(destNs, &svcAccDef)
			dstPod, _ := td.CreatePod(destNs, podDef)
			td.CreateService(destNs, svcDef)

			// Expect it to be up and running in it's receiver namespace
			td.WaitForPodsRunningReady(destNs, 30*time.Second)

			// Get simple Pod definitions for the client
			svcAccDef, podDef, svcDef = td.SimplePodApp(SimplePodAppDef{
				name:      "client",
				namespace: sourceNs,
				command:   []string{"/bin/bash", "-c", "--"},
				args:      []string{"while true; do sleep 30; done;"},
				image:     "songrgg/alpine-debug",
			})

			td.CreateServiceAccount(sourceNs, &svcAccDef)
			srcPod, _ := td.CreatePod(sourceNs, podDef)
			td.CreateService(sourceNs, svcDef)

			// Expect it to be up and running in it's receiver namespace
			td.WaitForPodsRunningReady(sourceNs, 30*time.Second)

			// Deploy allow rule client->server
			httpRG, trafficTarget := td.CreateSimpleAllowPolicy(
				SimpleAllowPolicy{
					RouteGroupName:    "routes",
					TrafficTargetName: "test-target",

					SourceNamespace:      sourceNs,
					SourceSVCAccountName: "client",

					DestinationNamespace:      destNs,
					DestinationSvcAccountName: "server",
				})

			// Configs have to be put into a monitored NS, and osm-system can't be by cli
			td.CreateHTTPRouteGroup(sourceNs, httpRG)
			td.CreateTrafficTarget(sourceNs, trafficTarget)

			// All ready. Expect client to reach server
			// Need to get the pod though.
			WaitForRepeatedSuccess(func() bool {
				statusCode, _, err :=
					td.HTTPRequest(HTTPRequestDef{
						SourceNs:        srcPod.Namespace,
						SourcePod:       srcPod.Name,
						SourceContainer: "client", // We can do better

						Destination: fmt.Sprintf("%s.%s", dstPod.Name, dstPod.Namespace),

						HTTPUrl: "/",
						Port:    80,
					})

				if err != nil || statusCode != 200 {
					td.T.Logf("> REST req failed (status: %d) %v", statusCode, err)
					return false
				}
				td.T.Logf("> REST req succeeded: %d", statusCode)
				return true
			}, 5, 40*time.Second)
		})
	})

	// Cleanup when error
	AfterSuite(func() {
		td.Cleanup()
	})
})
