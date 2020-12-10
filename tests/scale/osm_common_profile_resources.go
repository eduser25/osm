package scale

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/openservicemesh/osm/tests/framework"
)

const (
	defaultFilename = "results.txt"
)

// Returns the OSM grafana dashboards of interest to save after the test
func getOSMGrafanaSaveDashboards() []GrafanaPanel {
	return []GrafanaPanel{
		{
			Filename:  "cpu",
			Dashboard: MeshDetails,
			Panel:     CPUPanel,
		},
		{
			Filename:  "mem",
			Dashboard: MeshDetails,
			Panel:     MemRSSPanel,
		},
	}
}

// Returns labels to select OSM controller and OSM-installed Prometheus.
func getOSMTrackResources() []TrackedLabel {
	return []TrackedLabel{
		{
			Namespace: Td.OsmNamespace,
			Label: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": OsmControllerAppLabel,
				},
			},
		},
		{
			Namespace: Td.OsmNamespace,
			Label: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": OsmPrometheusAppLabel,
				},
			},
		},
	}
}

// Get common outputs we are interested to print in
func getOSMTestOutputFiles() []*os.File {
	fName := Td.GetTestFile(defaultFilename)
	f, err := os.Create(fName)
	if err != nil {
		fmt.Printf("Failed to open file: %v", err)
		return nil
	}

	return []*os.File{
		f,
		os.Stdout,
	}
}
