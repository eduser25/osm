package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
)

func TestOneShot(t *testing.T) {
	clusterName := "osm-e2e"
	t.Log("Creating kind cluster", clusterName)
	provider := cluster.NewProvider()
	if err := provider.Create(clusterName); err != nil {
		t.Fatal("error creating cluster:", err)
	}
	defer func() {
		t.Log("Deleting kind cluster", clusterName)
		if err := provider.Delete(clusterName, clientcmd.RecommendedHomeFile); err != nil {
			t.Error("error deleting cluster:", err)
		}
	}()

	t.Log("Getting image data")
	imageNames := []string{
		"bookstore",
		"bookbuyer",
		"bookthief",
		"bookwarehouse",
		"osm-controller",
		"init",
	}
	imageRegistry := "localhost:5000"
	imageTag := "testing"
	docker, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal("error creating docker client:", err)
	}
	var imageIDs []string
	for _, name := range imageNames {
		imageName := fmt.Sprintf("%s/%s:%s", imageRegistry, name, imageTag)
		imageIDs = append(imageIDs, imageName)
	}
	imageData, err := docker.ImageSave(context.TODO(), imageIDs)
	if err != nil {
		t.Fatal("error getting image data:", err)
	}
	defer imageData.Close()
	nodes, err := provider.ListNodes(clusterName)
	if err != nil {
		t.Fatal("error listing kind nodes:", err)
	}
	for _, n := range nodes {
		t.Log("Loading images onto node", n)
		if err := nodeutils.LoadImageArchive(n, imageData); err != nil {
			t.Fatal("error loading image:", err)
		}
	}

	// TODO: refactor install to be able to call it directly here vs. exec-ing CLI.
	t.Log("Installing control plane")
	controlPlaneNS := "osm-system"
	osmInstall := exec.Command(filepath.FromSlash("../../bin/osm"), "install",
		"--container-registry="+imageRegistry,
		"--osm-image-tag="+imageTag,
		"--namespace="+controlPlaneNS,
	)
	osmInstallStdout := bytes.NewBuffer(nil)
	osmInstallStderr := bytes.NewBuffer(nil)
	osmInstall.Stdout = osmInstallStdout
	osmInstall.Stderr = osmInstallStderr
	if err := osmInstall.Run(); err != nil {
		t.Log("error running osm install:", err)
		t.Logf("stdout: %s\n", osmInstallStdout)
		t.Logf("stderr: %s\n", osmInstallStderr)
		t.FailNow()
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := clientConfig.ClientConfig()
	if err != nil {
		t.Fatal("error creating kube client:", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal("error creating kube clientset:", err)
	}

	bookbuyerNS := "bookbuyer"
	bookthiefNS := "bookthief"
	bookstoreNS := "bookstore"
	bookwarehouseNS := "bookwarehouse"
	for _, name := range []string{bookbuyerNS, bookthiefNS, bookstoreNS, bookwarehouseNS} {
		_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			t.Log("Creating namespace", name)
			_, err := clientset.CoreV1().Namespaces().Create(context.TODO(),
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
				},
				metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("error creating namespace %s: %v\n", name, err)
			}
		} else if err != nil {
			t.Fatalf("error getting app namespace %s: %T %v\n", name, err, err)
		}

		// TODO: refactor namespace add to be able to call it directly here vs. exec-ing CLI.
		t.Log("Adding namespace", name, "to the control plane")
		osmNamespaceAdd := exec.Command(filepath.FromSlash("../../bin/osm"), "namespace", "add", name,
			"--enable-sidecar-injection",
			"--namespace="+controlPlaneNS,
		)
		stdout := bytes.NewBuffer(nil)
		stderr := bytes.NewBuffer(nil)
		osmNamespaceAdd.Stdout = stdout
		osmNamespaceAdd.Stderr = stderr
		if err := osmNamespaceAdd.Run(); err != nil {
			t.Log("error running osm namespace add:", err)
			t.Logf("stdout: %s\n", stdout)
			t.Logf("stderr: %s\n", stderr)
			t.FailNow()
		}
	}

	controlPlanePodsReadyTimeout := 90 * time.Second
	t.Logf("Waiting for %s for control plane pods to become ready", controlPlanePodsReadyTimeout)
	if err := wait.PollImmediate(5*time.Second, controlPlanePodsReadyTimeout, func() (bool, error) {
		pods, err := clientset.CoreV1().Pods(controlPlaneNS).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		ready := true
		if len(pods.Items) == 0 {
			return false, nil
		}
		for _, pod := range pods.Items {
			t.Log("Pod", pod.Name, "is", pod.Status.Phase)
			if pod.Status.Phase != corev1.PodRunning {
				ready = false
			}
		}
		return ready, nil
	}); err != nil {
		t.Fatal("error waiting for control plane pods ready:", err)
	}
	t.Log("Control plane pods ready")

	t.Log("Deploying apps")
	deployApps := exec.Command("bash", filepath.FromSlash("demo/deploy-apps.sh"))
	deployApps.Dir = filepath.FromSlash("../..")
	deployAppsStdout := bytes.NewBuffer(nil)
	deployAppsStderr := bytes.NewBuffer(nil)
	deployApps.Stdout = deployAppsStdout
	deployApps.Stderr = deployAppsStderr
	if err := deployApps.Run(); err != nil {
		t.Log("error running deploy-apps.sh:", err)
		t.Logf("stdout: %s\n", deployAppsStdout)
		t.Logf("stderr: %s\n", deployAppsStderr)
		t.FailNow()
	}

	t.Log("Deploying SMI policies")
	deployPolicies := exec.Command("bash", filepath.FromSlash("demo/deploy-smi-policies.sh"))
	deployPolicies.Dir = filepath.FromSlash("../..")
	deployPoliciesStdout := bytes.NewBuffer(nil)
	deployPoliciesStderr := bytes.NewBuffer(nil)
	deployPolicies.Stdout = deployPoliciesStdout
	deployPolicies.Stderr = deployPoliciesStderr
	if err := deployPolicies.Run(); err != nil {
		t.Log("error running deploy-smi-policies.sh:", err)
		t.Logf("stdout: %s\n", deployPoliciesStdout)
		t.Logf("stderr: %s\n", deployPoliciesStderr)
		t.FailNow()
	}

	// TODO: refactor maestro to be able to call it directly here vs. exec-ing go run.
	t.Log("Running maestro")
	maestro := exec.Command("go", "run", filepath.FromSlash("../../ci/cmd"))
	maestro.Env = append(maestro.Env,
		"HOME="+os.Getenv("HOME"),
		"CI_MAX_WAIT_FOR_POD_TIME_SECONDS=60",
		"CI_WAIT_FOR_OK_SECONDS=60",
	)
	maestroStdout := bytes.NewBuffer(nil)
	maestroStderr := bytes.NewBuffer(nil)
	maestro.Stdout = maestroStdout
	maestro.Stderr = maestroStderr
	if err := maestro.Run(); err != nil {
		t.Log("error running maestro:", err)
		t.Logf("stdout: %s\n", maestroStdout)
		t.Logf("stderr: %s\n", maestroStderr)
		t.FailNow()
	}
	t.Log("Maestro succeeded")
}
