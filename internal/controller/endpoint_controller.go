/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"github.com/MDZZ110/ai-manager/internal/config"
	"github.com/MDZZ110/ai-manager/internal/utils"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"

	aimanageriov1alpha1 "github.com/MDZZ110/ai-manager/api/v1alpha1"
)

// EndpointReconciler reconciles a Endpoint object
type EndpointReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *config.Config
}

//+kubebuilder:rbac:groups=ai.manager.io,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ai.manager.io,resources=endpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ai.manager.io,resources=endpoints/finalizers,verbs=update
//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Endpoint object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (e *EndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	endpoint := &aimanageriov1alpha1.Endpoint{}

	if err := e.Get(ctx, req.NamespacedName, endpoint); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if endpoint.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsFinalizer(endpoint.GetFinalizers(), aimanageriov1alpha1.EndpointFinalizer) {
			e.SetFinalizers(endpoint)
			if err := e.Update(ctx, endpoint); err != nil {
				logger.V(0).Info("unable to add finalizer on endpoint resource", "name", endpoint.Name, "err", err.Error())
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		logger.V(0).Info("receive deletion request", "name", endpoint.Name)
		if utils.ContainsFinalizer(endpoint.GetFinalizers(), aimanageriov1alpha1.EndpointFinalizer) {
			err := e.ReleaseResources(ctx, req.NamespacedName, logger)
			if err != nil {
				return ctrl.Result{}, err
			}

			e.RemoveFinalizers(endpoint)
			if err := e.Update(ctx, endpoint); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	matchLabels := map[string]string{
		aimanageriov1alpha1.LabelApp:       endpoint.Name,
		aimanageriov1alpha1.LabelChatWebUI: "true",
	}

	desiredDeployment := e.getDesiredWorkerDeployment(endpoint, matchLabels)
	desiredDeployment.SetOwnerReferences(GetEndpointOwnerReference(endpoint))
	err := e.CreateOrUpdateDeployment(ctx, desiredDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (e *EndpointReconciler) CreateOrUpdateDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current = &appsv1.Deployment{}
		err := e.Get(ctx, client.ObjectKeyFromObject(desired), current)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return e.Create(ctx, desired)
		}

		mergeMetadata(&desired.ObjectMeta, &current.ObjectMeta)
		return e.Update(ctx, desired)
	})
}

func GetEndpointOwnerReference(endpoint *aimanageriov1alpha1.Endpoint) []metav1.OwnerReference {
	isController := true
	controllerRef := []metav1.OwnerReference{
		{
			APIVersion: endpoint.APIVersion,
			Kind:       endpoint.Kind,
			Name:       endpoint.Name,
			UID:        endpoint.UID,
			Controller: &isController,
		},
	}

	return utils.MergeOwnerReferences(endpoint.GetOwnerReferences(), controllerRef)
}

// SetupWithManager sets up the controller with the Manager.
func (e *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimanageriov1alpha1.Endpoint{}).
		Complete(e)
}

func (e *EndpointReconciler) SetFinalizers(endpoint *aimanageriov1alpha1.Endpoint) {
	finalizers := endpoint.GetFinalizers()
	finalizers = append(finalizers, aimanageriov1alpha1.EndpointFinalizer)
	endpoint.SetFinalizers(finalizers)
}

func (e *EndpointReconciler) RemoveFinalizers(endpoint *aimanageriov1alpha1.Endpoint) {
	finalizers := endpoint.GetFinalizers()
	for i := 0; i < len(finalizers); i++ {
		if finalizers[i] == aimanageriov1alpha1.EndpointFinalizer {
			newFinalizers := append(finalizers[:i], finalizers[i+1:]...)
			endpoint.SetFinalizers(newFinalizers)
			return
		}
	}
}

func (e *EndpointReconciler) ReleaseResources(ctx context.Context, nn types.NamespacedName, log logr.Logger) (err error) {
	return
}

func (e *EndpointReconciler) getDesiredWorkerDeployment(endpoint *aimanageriov1alpha1.Endpoint, matchLabels map[string]string) (workerDeployment *appsv1.Deployment) {
	container := e.createContainer(endpoint)
	workerDeployment = &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", aimanageriov1alpha1.ChatWebName, endpoint.Name),
			Namespace: endpoint.Namespace,
			Labels:    matchLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &endpoint.Spec.WebSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: matchLabels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{},
					NodeSelector:    endpoint.Spec.WebSpec.NodeSelector,
					Tolerations:     endpoint.Spec.WebSpec.Tolerations,
					Affinity:        endpoint.Spec.WebSpec.Affinity,
				},
			},
		},
	}

	workerDeployment.Spec.Template.Spec.Containers = []corev1.Container{container}
	return
}

func (e *EndpointReconciler) getWebServiceImage(endpoint *aimanageriov1alpha1.Endpoint) (webImage string) {
	if endpoint.Spec.WebSpec.Image != "" {
		return endpoint.Spec.WebSpec.Image
	}

	if strings.HasSuffix(e.Config.Registry, "/") {
		return fmt.Sprintf("%s%s:%s", e.Config.Registry, aimanageriov1alpha1.DefaultChatWebImage, aimanageriov1alpha1.DefaultChatWebImageTag)
	}

	return fmt.Sprintf("%s/%s:%s", e.Config.Registry, aimanageriov1alpha1.DefaultChatWebImage, aimanageriov1alpha1.DefaultChatWebImageTag)
}

func (e *EndpointReconciler) createContainer(endpoint *aimanageriov1alpha1.Endpoint) (container corev1.Container) {
	container = corev1.Container{
		Name:            endpoint.Name,
		Image:           e.getWebServiceImage(endpoint),
		ImagePullPolicy: endpoint.Spec.WebSpec.ImagePullPolicy,
		Ports:           createWebServicePorts(endpoint),
	}

	container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	container.TerminationMessagePath = corev1.TerminationMessagePathDefault
	container.SecurityContext = &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		AllowPrivilegeEscalation: pointer.Bool(false),
		RunAsNonRoot:             pointer.Bool(true),
	}

	container.Command = []string{}

	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz",
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 8443,
				},
				Scheme: corev1.URISchemeHTTPS,
			},
		},
		InitialDelaySeconds: 2,
		PeriodSeconds:       5,
		FailureThreshold:    3,
		SuccessThreshold:    1,
		TimeoutSeconds:      1,
	}

	container.Resources = endpoint.Spec.WebSpec.Resources
	container.Env = []corev1.EnvVar{
		{
			Name:  "OPENAI_API_KEY",
			Value: endpoint.Spec.WebSpec.OpenAiKey,
		},
		{
			Name:  "CODE",
			Value: endpoint.Spec.WebSpec.Password,
		},
	}
	return
}

func createWebServicePorts(endpoint *aimanageriov1alpha1.Endpoint) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          endpoint.Spec.WebSpec.PortName,
			ContainerPort: aimanageriov1alpha1.WebContainerPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
}

func (e *EndpointReconciler) getDesiredService(endpoint *aimanageriov1alpha1.Endpoint, labels map[string]string) (svc *corev1.Service) {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: endpoint.Name,
			Labels: map[string]string{
				aimanageriov1alpha1.LabelApp: endpoint.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       endpoint.Spec.WebSpec.PortName,
					Port:       endpoint.Spec.WebSpec.ServicePort,
					TargetPort: intstr.FromString(endpoint.Spec.WebSpec.PortName),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}
