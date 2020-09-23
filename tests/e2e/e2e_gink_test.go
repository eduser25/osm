package e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("Test and deploy a simple mesh", func() {
	var td OsmTestData

	Context("Test OSM testing APIs", func() {
		var ns []string = []string{"sender", "receiver"}

		It("Initializes Test flags, data, and cluster configuration", func() {
			td = InitTestData(GinkgoT())
		})

		// It("Updates images on container registry", func() {
		// 	// Do we want this on test basis?
		// })

		It("Installs OSM", func() {
			td.InstallOSM(td.GetTestInstallOpts())
		})

		It("Create test namespaces", func() {
			for _, n := range ns {
				td.CreateNs(n, nil)
			}
		})

		// More steps coming

		It("Deletes Namespace deletion upon test finish", func() {
			for _, n := range ns {
				td.DeleteNs(n)
			}
			td.WaitForNamespacesDeleted(ns, 30*time.Second)
		})
	})

	// Cleanup when error
	AfterSuite(func() {
		td.Cleanup()
	})
})
