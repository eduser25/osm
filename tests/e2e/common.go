package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/kind/pkg/cluster"
)

// OsmTestData stores common state and variables and flags of a test
type OsmTestData struct {
	T GinkgoTInterface // for common test logging

	cleanupTest    bool // Cleanup test-related resources once finished
	waitForCleanup bool // Forces application to wait for deletion of resources

	// OSM install-time variables
	osmMeshName        string
	ctrRegistry        string
	ctrRegistrySercret string

	kindCluster        bool   // Create and use a kind cluster
	clusterName        string // Kind cluster name (used if kindCluster)
	cleanupKindCluster bool   // Cleanup kind cluster upon test finish

	// Cluster handles and rest config
	RestConfig      *rest.Config
	Client          *kubernetes.Clientset
	SmiClients      *SmiClients
	ClusterProvider *cluster.Provider // provider, used when kindCluster is used

	// Managed resources, used to track what gets cleaned up after the test
	Namespaces map[string]bool
}

func registerFlags(td *OsmTestData) {
	flag.BoolVar(&td.cleanupTest, "cleanupTest", true, "Cleanup test resources when done")
	flag.BoolVar(&td.waitForCleanup, "waitForCleanup", false, "Wait for effective deletion of resources")

	flag.BoolVar(&td.kindCluster, "kindCluster", false, "Creates kind cluster")
	flag.StringVar(&td.clusterName, "kindClusterName", "osm-e2e", "Name of the Kind cluster to be created")
	flag.BoolVar(&td.cleanupKindCluster, "cleanupKindCluster", true, "Cleanup kind cluster upon exit")

	flag.StringVar(&td.ctrRegistry, "containerRegistry", os.Getenv("CTR_REGISTRY"), "Container registry")
	flag.StringVar(&td.ctrRegistrySercret, "containerRegistrySecret", os.Getenv("CTR_REGISTRY_PASSWORD"), "Container registry secret")

	flag.StringVar(&td.osmMeshName, "meshName", func() string {
		tmp := os.Getenv("K8S_NAMESPACE")
		if len(tmp) != 0 {
			return tmp
		}
		return "osm-system"
	}(), "OSM mesh name")

	if len(td.ctrRegistry) == 0 {
		td.T.Log("warn: did not read any container registry")
	}
}

// InitTestData Initializes the test structures
func (td *OsmTestData) InitTestData(t GinkgoTInterface) {
	// Parse Generic test flags
	td.T = t
	td.Namespaces = make(map[string]bool)

	if td.kindCluster {
		td.T.Logf("Creating kind cluster: %s", td.clusterName)
		td.ClusterProvider = cluster.NewProvider()
		if err := td.ClusterProvider.Create(td.clusterName); err != nil {
			td.T.Fatalf("error creating cluster: %v", err)
		}
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	kubeConfig, err := clientConfig.ClientConfig()
	if err != nil {
		fmt.Println("error loading kube config:", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		fmt.Println("error in getting access to K8S")
		os.Exit(1)
	}

	td.RestConfig = kubeConfig
	td.Client = clientset

	td.InitSMIClients()
}

// InstallOSMOpts describes install options for OSM
type InstallOSMOpts struct {
	controlPlaneNS          string
	containerRegistryLoc    string
	containerRegistrySecret string
	osmImagetag             string
	deployGrafana           bool
	deployPrometheus        bool
	deployJaeger            bool
}

// GetTestInstallOpts returns Install opts based on test flags
func (td *OsmTestData) GetTestInstallOpts() InstallOSMOpts {
	return InstallOSMOpts{
		controlPlaneNS:          td.osmMeshName,
		containerRegistryLoc:    td.ctrRegistry,
		containerRegistrySecret: td.ctrRegistrySercret,
		osmImagetag:             "latest",
		deployGrafana:           false,
		deployPrometheus:        false,
		deployJaeger:            false,
	}
}

// InstallOSM installs OSM. Right now relies on externally calling the binary and a subset of possible opts
// TODO: refactor install to be able to call it directly here vs. exec-ing CLI.
func (td *OsmTestData) InstallOSM(instOpts InstallOSMOpts) {
	td.T.Log("Installing OSM")
	var args []string

	args = append(args, "install",
		"--container-registry="+instOpts.containerRegistryLoc,
		"--osm-image-tag="+instOpts.osmImagetag,
		"--namespace="+instOpts.controlPlaneNS,
		"--enable-debug-server")

	if len(instOpts.containerRegistrySecret) != 0 {
		args = append(args, "--container-registry-secret="+instOpts.containerRegistrySecret)
	}

	args = append(args, fmt.Sprintf("--enable-prometheus=%v", instOpts.deployPrometheus))
	args = append(args, fmt.Sprintf("--enable-grafana=%v", instOpts.deployGrafana))
	args = append(args, fmt.Sprintf("--deploy-jaeger=%v", instOpts.deployJaeger))

	// Add OSM namespace to cleanup namespaces
	td.Namespaces[instOpts.controlPlaneNS] = true

	td.RunLocal(filepath.FromSlash("../../bin/osm"), args)
}

// AddNsToMesh Adds monitored namespaces to the OSM mesh
func (td *OsmTestData) AddNsToMesh(ns ...string) {
	td.T.Logf("Adding Namespaces [+%s] to the mesh", ns)

	for _, namespace := range ns {
		args := []string{"namespace", "add", namespace, "--enable-sidecar-injection", "--namespace=" + td.osmMeshName}
		td.RunLocal(filepath.FromSlash("../../bin/osm"), args)
	}
}

// CreateNs Creates a test NS
func (td *OsmTestData) CreateNs(nsName string, labels map[string]string) error {
	if labels == nil {
		labels = map[string]string{}
	}

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nsName,
			Namespace: "",
			Labels:    labels,
		},
		Status: corev1.NamespaceStatus{},
	}

	td.T.Logf("Creating namespace %v", nsName)
	_, err := td.Client.CoreV1().Namespaces().Create(context.Background(), namespaceObj, metav1.CreateOptions{})
	if err != nil {
		td.T.Fatalf("Failed to create namespace: %v", err)
	}
	td.Namespaces[nsName] = true

	return nil
}

// DeleteNs deletes a test NS
func (td *OsmTestData) DeleteNs(nsName string) error {
	var backgroundDelete metav1.DeletionPropagation = metav1.DeletePropagationBackground

	td.T.Logf("Deleting namespace %v", nsName)
	err := td.Client.CoreV1().Namespaces().Delete(context.Background(), nsName, metav1.DeleteOptions{PropagationPolicy: &backgroundDelete})
	if err != nil {
		td.T.Logf("Failed to delete namespace: %v", err)
	}
	delete(td.Namespaces, nsName)

	return err
}

// WaitForNamespacesDeleted waits for the namespaces to be deleted.
// Taken from https://github.com/kubernetes/kubernetes/blob/master/test/e2e/framework/util.go#L258
func (td *OsmTestData) WaitForNamespacesDeleted(namespaces []string, timeout time.Duration) error {
	ginkgo.By(fmt.Sprintf("Waiting for namespaces %+v to vanish", namespaces))
	nsMap := map[string]bool{}
	for _, ns := range namespaces {
		nsMap[ns] = true
	}
	//Now POLL until all namespaces have been eradicated.
	return wait.Poll(2*time.Second, timeout,
		func() (bool, error) {
			nsList, err := td.Client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			for _, item := range nsList.Items {
				if _, ok := nsMap[item.Name]; ok {
					return false, nil
				}
			}
			return true, nil
		})
}

// RunLocal Executes command on local
func (td *OsmTestData) RunLocal(path string, args []string) {
	cmd := exec.Command(path, args...)
	cmd.Stdout = bytes.NewBuffer(nil)
	cmd.Stderr = bytes.NewBuffer(nil)

	td.T.Logf("running '%s %s'", path, strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		td.T.Logf("error running cmd '%s %s' : %v",
			path, strings.Join(args, " "), err)
		td.T.Logf("stdout: %s\n", cmd.Stdout)
		td.T.Logf("stderr: %s\n", cmd.Stderr)
		td.T.FailNow()
	}
}

// RunRemote runs command in remote container
func (td *OsmTestData) RunRemote(
	ns string, podName string, containerName string,
	command string,
	stdin io.Reader, stdout io.Writer, stderr io.Writer) error {

	req := td.Client.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(ns).SubResource("exec")

	option := &v1.PodExecOptions{
		Command:   strings.Fields(command),
		Container: containerName,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}
	if stdin == nil {
		option.Stdin = false
	}
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	req.VersionedParams(
		option,
		runtime.NewParameterCodec(scheme),
	)
	exec, err := remotecommand.NewSPDYExecutor(td.RestConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}

	return nil
}

// WaitForPodsRunningReady waits for a pods on an NS to be running and ready
func (td *OsmTestData) WaitForPodsRunningReady(ns string, timeout time.Duration) error {
	td.T.Logf("Wait for pods ready in ns [%s]...", ns)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(2 * time.Second) {
		pods, err := td.Client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil || len(pods.Items) == 0 {
			continue
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return nil
				}
			}
		}
	}

	err := fmt.Errorf("Not all pods were Running & Ready in NS %s after %v", ns, timeout)
	td.T.Fatalf("%v", err)
	return err
}

// Cleanup is Used to cleanup resorces once the test is done
func (td *OsmTestData) Cleanup() {
	// In-cluster Test resources cleanup(namespace, crds, specs and whatnot) here
	if td.cleanupTest {
		var nsList []string
		for ns := range td.Namespaces {
			td.DeleteNs(ns)
			nsList = append(nsList, ns)
		}

		if td.waitForCleanup {
			err := td.WaitForNamespacesDeleted(nsList, 30*time.Second)
			if err != nil {
				td.T.Fatalf("Could not confirm all namespace deletion in time: %v", err)
			}
		}
	}

	// Kind cluster deletion, if needed
	if td.kindCluster && td.cleanupKindCluster {
		td.T.Logf("Deleting kind cluster: %s", td.clusterName)
		if err := td.ClusterProvider.Delete(td.clusterName, clientcmd.RecommendedHomeFile); err != nil {
			td.T.Errorf("error deleting cluster: %v", err)
		}
	}
}
