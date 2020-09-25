package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/client"
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
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
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
	osmImageTag        string

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

	flag.StringVar(&td.osmImageTag, "osmImageTag", "latest", "OSM image tag")

	flag.StringVar(&td.osmMeshName, "meshName", func() string {
		tmp := os.Getenv("K8S_NAMESPACE")
		if len(tmp) != 0 {
			return tmp
		}
		return "osm-system"
	}(), "OSM mesh name")
}

// InitTestData Initializes the test structures
func (td *OsmTestData) InitTestData(t GinkgoTInterface) {
	td.T = t
	td.Namespaces = make(map[string]bool)

	if len(td.ctrRegistry) == 0 {
		td.T.Log("warn: did not read any container registry")
	}

	if td.kindCluster {
		td.ClusterProvider = cluster.NewProvider()
		td.T.Logf("Creating local kind cluster")
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
		osmImagetag:             td.osmImageTag,
		deployGrafana:           false,
		deployPrometheus:        false,
		deployJaeger:            false,
	}
}

// InstallOSM installs OSM. Right now relies on externally calling the binary and a subset of possible opts
// TODO: refactor install to be able to call it directly here vs. exec-ing CLI.
func (td *OsmTestData) InstallOSM(instOpts InstallOSMOpts) {
	if td.kindCluster {
		td.T.Log("Getting image data")
		imageNames := []string{
			"osm-controller",
			"init",
		}
		docker, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
		if err != nil {
			td.T.Fatal("error creating docker client:", err)
		}
		var imageIDs []string
		for _, name := range imageNames {
			imageName := fmt.Sprintf("%s/%s:%s", td.ctrRegistry, name, td.osmImageTag)
			imageIDs = append(imageIDs, imageName)
		}
		imageData, err := docker.ImageSave(context.TODO(), imageIDs)
		if err != nil {
			td.T.Fatal("error getting image data:", err)
		}
		defer imageData.Close()
		nodes, err := td.ClusterProvider.ListNodes(td.clusterName)
		if err != nil {
			td.T.Fatal("error listing kind nodes:", err)
		}
		for _, n := range nodes {
			td.T.Log("Loading images onto node", n)
			if err := nodeutils.LoadImageArchive(n, imageData); err != nil {
				td.T.Fatal("error loading image:", err)
			}
		}
	}

	td.T.Log("Installing OSM")
	var args []string

	// Add OSM namespace to cleanup namespaces, in case the test can't init
	td.Namespaces[instOpts.controlPlaneNS] = true

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

	td.RunLocal(filepath.FromSlash("../../bin/osm"), args)
}

// AddNsToMesh Adds monitored namespaces to the OSM mesh
func (td *OsmTestData) AddNsToMesh(sidecardInject bool, ns ...string) {
	td.T.Logf("Adding Namespaces [+%s] to the mesh", ns)
	for _, namespace := range ns {
		args := []string{"namespace", "add", namespace}
		if sidecardInject {
			args = append(args, "--enable-sidecar-injection")
		}

		args = append(args, "--namespace="+td.osmMeshName)
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
	command string) (string, string, error) {
	var stdin, stdout, stderr bytes.Buffer

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

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	req.VersionedParams(
		option,
		runtime.NewParameterCodec(scheme),
	)
	exec, err := remotecommand.NewSPDYExecutor(td.RestConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  &stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
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

// SuccessFunction is a simple definiton for a success function.
// True as success, false otherwise
type SuccessFunction func() bool

// WaitForRepeatedSuccess runs and expects a certain result for a certain operation a set number of consecutive times
// over a set amount of time.
func WaitForRepeatedSuccess(f SuccessFunction, minItForSuccess int, maxWaitTime time.Duration) bool {
	iterations := 0
	startTime := time.Now()

	for time.Since(startTime) < maxWaitTime {
		if f() {
			iterations++
			if iterations >= minItForSuccess {
				return true
			}
		} else {
			iterations = 0
		}
		time.Sleep(time.Second)
	}
	return false
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
