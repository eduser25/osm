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

			// To wait for all
			var wg sync.WaitGroup

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
		})
	})
})
