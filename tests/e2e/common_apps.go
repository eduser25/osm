package e2e

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// CreateServiceAccount creates a service account
func (td *OsmTestData) CreateServiceAccount(ns string, svcAccount *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	svcAc, err := td.Client.CoreV1().ServiceAccounts(ns).Create(context.Background(), svcAccount, metav1.CreateOptions{})
	if err != nil {
		err := fmt.Errorf("Could not create Service Account: %v", err)
		td.T.Fatalf("%v", err)
		return nil, err
	}
	return svcAc, nil
}

// CreatePod creates a pod
func (td *OsmTestData) CreatePod(ns string, pod corev1.Pod) (*corev1.Pod, error) {
	podRet, err := td.Client.CoreV1().Pods(ns).Create(context.Background(), &pod, metav1.CreateOptions{})
	if err != nil {
		err := fmt.Errorf("Could not create Pod: %v", err)
		td.T.Fatalf("%v", err)
		return nil, err
	}
	return podRet, nil
}

// CreateDeployment creates a pod
func (td *OsmTestData) CreateDeployment(ns string, deployment appsv1.Deployment) (*appsv1.Deployment, error) {
	deploymentRet, err := td.Client.AppsV1().Deployments(ns).Create(context.Background(), &deployment, metav1.CreateOptions{})
	if err != nil {
		err := fmt.Errorf("Could not create Deployment: %v", err)
		td.T.Fatalf("%v", err)
		return nil, err
	}
	return deploymentRet, nil
}

// CreateService a service
func (td *OsmTestData) CreateService(ns string, svc corev1.Service) (*corev1.Service, error) {
	sv, err := td.Client.CoreV1().Services(ns).Create(context.Background(), &svc, metav1.CreateOptions{})
	if err != nil {
		err := fmt.Errorf("Could not create Service: %v", err)
		td.T.Fatalf("%v", err)
		return nil, err
	}
	return sv, nil
}

// Strightforward Templates below
// We might want to decouple/interface/parametrize more things in the future. This can get complex very quickly; we might
// want to have smaller apis to decouple getting to create individual service accounts, pod defs, deployments, etc...

// SimplePodAppDef defines some parametrization to create a pod-based application from template
type SimplePodAppDef struct {
	namespace string
	name      string
	image     string
	command   []string
	args      []string
}

// SimplePodApp creates a template for a Pod-based app definition for testing
func (td *OsmTestData) SimplePodApp(def SimplePodAppDef) (corev1.ServiceAccount, corev1.Pod, corev1.Service) {
	serviceAccountDefinition := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: def.name,
		},
	}

	podDefinition := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: def.name,
			Labels: map[string]string{
				"app": def.name,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: def.name,
			Containers: []corev1.Container{
				{
					Name:            def.name,
					Image:           def.image,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
			},
		},
	}

	if td.AreRegistryCredsPresent() {
		podDefinition.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: registrySecretName,
			},
		}
	}
	if def.command != nil && len(def.command) > 0 {
		podDefinition.Spec.Containers[0].Command = def.command
	}
	if def.args != nil && len(def.args) > 0 {
		podDefinition.Spec.Containers[0].Args = def.args
	}

	serviceDefinition := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: def.name,
			Labels: map[string]string{
				"app": def.name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": def.name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}

	return serviceAccountDefinition, podDefinition, serviceDefinition
}

// SimpleDeploymentAppDef defines some parametrization to create a deployment-based application from template
type SimpleDeploymentAppDef struct {
	namespace    string
	name         string
	image        string
	replicaCount int32
	command      []string
	args         []string
}

// SimpleDeploymentApp creates a template for a deployment-based app definition for testing
func (td *OsmTestData) SimpleDeploymentApp(def SimpleDeploymentAppDef) (corev1.ServiceAccount, appsv1.Deployment, corev1.Service) {
	serviceAccountDefinition := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      def.name,
			Namespace: def.namespace,
		},
	}

	// Required, as replica count is a pointer to distinguish between 0 and not specified
	replicaCountExplicitDeclaration := def.replicaCount

	deploymentDefinition := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      def.name,
			Namespace: def.namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaCountExplicitDeclaration,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": def.name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": def.name,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: def.name,
					Containers: []corev1.Container{
						{
							Name:            def.name,
							Image:           def.image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}

	if td.AreRegistryCredsPresent() {
		deploymentDefinition.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: registrySecretName,
			},
		}
	}

	if def.command != nil && len(def.command) > 0 {
		deploymentDefinition.Spec.Template.Spec.Containers[0].Command = def.command
	}
	if def.args != nil && len(def.args) > 0 {
		deploymentDefinition.Spec.Template.Spec.Containers[0].Args = def.args
	}

	serviceDefinition := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      def.name,
			Namespace: def.namespace,
			Labels: map[string]string{
				"app": def.name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": def.name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}

	return serviceAccountDefinition, deploymentDefinition, serviceDefinition
}
