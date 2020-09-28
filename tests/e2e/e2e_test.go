package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/client"
	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	smiSpecs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	smiTrafficAccessClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	smiTrafficSpecClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
)

func estOneShot(t *testing.T) {
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

	appNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e",
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), appNS, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating namespace %s: %v", appNS, err)
	}
	// TODO: refactor namespace add to be able to call it directly here vs. exec-ing CLI.
	t.Log("Adding namespace", appNS.Name, "to the control plane")
	osmNamespaceAdd := exec.Command(filepath.FromSlash("../../bin/osm"), "namespace", "add", appNS.Name,
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
	serverName := "server"
	serverLabels := map[string]string{
		"app": serverName,
	}
	serverSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: serverName,
		},
	}
	_, err = clientset.CoreV1().ServiceAccounts(appNS.Name).Create(context.TODO(), serverSA, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating server service account:", err)
	}
	serverPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   serverName,
			Labels: serverLabels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: serverSA.Name,
			Containers: []corev1.Container{
				{
					Name:  serverName,
					Image: "kennethreitz/httpbin",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
					},
				},
			},
		},
	}
	_, err = clientset.CoreV1().Pods(appNS.Name).Create(context.TODO(), serverPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating server pod:", err)
	}
	serverSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   serverName,
			Labels: serverLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: serverLabels,
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}
	_, err = clientset.CoreV1().Services(appNS.Name).Create(context.TODO(), serverSvc, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating server service:", err)
	}
	clientName := "client"
	clientLabels := map[string]string{
		"app": clientName,
	}
	clientSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: clientName,
		},
	}
	_, err = clientset.CoreV1().ServiceAccounts(appNS.Name).Create(context.TODO(), clientSA, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating client service account:", err)
	}
	clientPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clientName,
			Labels: clientLabels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: clientSA.Name,
			Containers: []corev1.Container{
				{
					Name:  clientName,
					Image: "curlimages/curl:7.72.0",
					Args: []string{
						"-fv",
						"--no-progress-meter",
						"--max-time", "5",
						"--retry", "60",
						"--retry-delay", "1",
						"--retry-connrefused",
						"server/status/200",
					},
				},
			},
		},
	}
	_, err = clientset.CoreV1().Pods(appNS.Name).Create(context.TODO(), clientPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating client pod:", err)
	}
	clientSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clientName,
			Labels: clientLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: clientLabels,
			Ports: []corev1.ServicePort{
				{
					Port: 65535, // unused
				},
			},
		},
	}
	_, err = clientset.CoreV1().Services(appNS.Name).Create(context.TODO(), clientSvc, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating client service:", err)
	}

	t.Log("Deploying SMI policies")
	trafficSpec := &smiSpecs.HTTPRouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "routes",
		},
		Spec: smiSpecs.HTTPRouteGroupSpec{
			Matches: []smiSpecs.HTTPMatch{
				{
					Name:      "all",
					PathRegex: ".*",
					Methods:   []string{string(smiSpecs.HTTPRouteMethodAll)},
				},
			},
		},
	}
	smiSpecClientset, err := smiTrafficSpecClient.NewForConfig(config)
	if err != nil {
		t.Fatal("error creating traffic spec client:", err)
	}
	_, err = smiSpecClientset.SpecsV1alpha3().HTTPRouteGroups(appNS.Name).Create(context.TODO(), trafficSpec, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating HTTPRouteGroup:", err)
	}
	trafficTarget := &smiAccess.TrafficTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target",
		},
		Spec: smiAccess.TrafficTargetSpec{
			Destination: smiAccess.IdentityBindingSubject{
				Kind:      "ServiceAccount",
				Name:      serverSA.Name,
				Namespace: appNS.Name,
			},
			Rules: []smiAccess.TrafficTargetRule{
				{
					Kind:    "HTTPRouteGroup",
					Name:    trafficSpec.Name,
					Matches: []string{"all"},
				},
			},
			Sources: []smiAccess.IdentityBindingSubject{
				{
					Kind:      "ServiceAccount",
					Name:      clientSA.Name,
					Namespace: appNS.Name,
				},
			},
		},
	}
	smiAccessClientset, err := smiTrafficAccessClient.NewForConfig(config)
	if err != nil {
		t.Fatal("error creating traffic access client:", err)
	}
	_, err = smiAccessClientset.AccessV1alpha2().TrafficTargets(appNS.Name).Create(context.TODO(), trafficTarget, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("error creating HTTPRouteGroup:", err)
	}

	var client *corev1.Pod
	t.Logf("Waiting for client to finish")
	if err := wait.PollImmediateInfinite(5*time.Second, func() (bool, error) {
		client, err = clientset.CoreV1().Pods(appNS.Name).Get(context.TODO(), clientPod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		ctr := client.Status.ContainerStatuses[0]
		return ctr.State.Terminated != nil, nil
	}); err != nil {
		t.Fatal("error waiting for client pod:", err)
	}
	exitCode := client.Status.ContainerStatuses[0].State.Terminated.ExitCode
	switch exitCode {
	case 0:
		t.Log("client succeeded")
	default:
		t.Error("client failed with exit code", exitCode)
	}
}
