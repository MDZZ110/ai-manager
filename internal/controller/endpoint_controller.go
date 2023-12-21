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
	"github.com/mitchellh/hashstructure"
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
	"time"

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
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

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
	var err error
	logger := log.FromContext(ctx)
	endpoint := &aimanageriov1alpha1.Endpoint{}

	if err := e.Get(ctx, req.NamespacedName, endpoint); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	endpoint = e.SetDefaultValue(endpoint)
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
			err = e.Update(ctx, endpoint)
			if err != nil {
				logger.Error(err, "update endpoint failed", "endpointName", endpoint.Name)
			}

			return ctrl.Result{}, err
		}
	}

	matchLabels := map[string]string{
		aimanageriov1alpha1.LabelApp:       endpoint.Name,
		aimanageriov1alpha1.LabelChatWebUI: "true",
	}

	var hashInt64 uint64
	hashInt64, err = hashstructure.Hash(struct {
		Name               string
		EndpointGeneration int64
		Config             config.Config
	}{
		Name:               endpoint.Name,
		EndpointGeneration: endpoint.Generation,
		Config:             *e.Config,
	}, nil)
	if err != nil {
		logger.Error(err, "failed to calculate combined endpoint hash", "endpointName", endpoint.Name)
		return ctrl.Result{}, err
	}

	hash := fmt.Sprintf("%d", hashInt64)
	if endpoint.Status.Hash == hash {
		logger.V(0).Info("combinedHash not updated, skipping updating endpoint component",
			"oldHash", endpoint.Status.Hash,
			"newHash", hash,
			"endpointName", endpoint.Name)
		return ctrl.Result{}, nil
	}

	defer func() {
		endpoint.Status.Hash = hash
		endpoint.Status.LastTransitionTime = metav1.Time{
			Time: time.Now().UTC(),
		}

		if err != nil {
			endpoint.Status.Message = err.Error()
			endpoint.Status.Status = aimanageriov1alpha1.DeployedEndpointStatus
		} else {
			endpoint.Status.Status = aimanageriov1alpha1.FailedEndpointStatus
		}

		err = e.UpdateStatus(ctx, endpoint)
		if err != nil {
			logger.Error(err, "update endpoint status failed", "endpointName", endpoint.Name)
		}
	}()

	desiredDeployment := e.getDesiredWorkerDeployment(endpoint, matchLabels)
	err = ctrl.SetControllerReference(endpoint, desiredDeployment, e.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = e.CreateOrUpdateDeployment(ctx, desiredDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	desiredService := e.getDesiredService(endpoint, matchLabels)
	desiredService.SetOwnerReferences(GetEndpointOwnerReference(endpoint))
	err = e.CreateOrUpdateService(ctx, desiredService)
	if err != nil {
		logger.Error(err, "create or update chat web service failed", "endpointName", endpoint.Name)
		return ctrl.Result{}, err
	}

	desiredInference := e.getDesiredInference(endpoint)
	desiredInference.SetOwnerReferences(GetEndpointOwnerReference(endpoint))
	err = e.CreateOrUpdateInference(ctx, desiredInference)
	if err != nil {
		logger.Error(err, "create or update inference failed", "inferName", endpoint.Name)
	}

	return ctrl.Result{}, err
}

func (e *EndpointReconciler) SetDefaultValue(endpoint *aimanageriov1alpha1.Endpoint) *aimanageriov1alpha1.Endpoint {
	credSecretRef := corev1.LocalObjectReference{e.Config.CredSecretName}
	if len(endpoint.Spec.InferSpec.ImagePullSecrets) == 0 {
		endpoint.Spec.InferSpec.ImagePullSecrets = append(endpoint.Spec.InferSpec.ImagePullSecrets, credSecretRef)
	}

	if len(endpoint.Spec.WebSpec.ImagePullSecrets) == 0 {
		endpoint.Spec.WebSpec.ImagePullSecrets = append(endpoint.Spec.WebSpec.ImagePullSecrets, credSecretRef)
	}

	return endpoint
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

func (e *EndpointReconciler) getPodMetadata(endpoint *aimanageriov1alpha1.Endpoint, matchLabels map[string]string) (map[string]string, map[string]string) {
	podAnnotations := make(map[string]string)
	podLabels := make(map[string]string)
	for k, v := range matchLabels {
		podLabels[k] = v
	}

	if endpoint.Spec.WebSpec.PodMetadata != nil {
		for k, v := range endpoint.Spec.WebSpec.PodMetadata.Labels {
			podLabels[k] = v
		}

		for k, v := range endpoint.Spec.WebSpec.PodMetadata.Annotations {
			podAnnotations[k] = v
		}
	}

	return podAnnotations, podLabels
}

func (e *EndpointReconciler) getDesiredInference(endpoint *aimanageriov1alpha1.Endpoint) *aimanageriov1alpha1.Inference {
	desiredInference := &aimanageriov1alpha1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name:        endpoint.Name,
			Namespace:   endpoint.Namespace,
			Labels:      endpoint.Labels,
			Annotations: endpoint.Annotations,
		},
		Spec: endpoint.Spec.InferSpec,
	}

	return desiredInference
}

func (e *EndpointReconciler) getDesiredWorkerDeployment(endpoint *aimanageriov1alpha1.Endpoint, matchLabels map[string]string) (workerDeployment *appsv1.Deployment) {
	container := e.createContainer(endpoint)
	podAnnotations, podLabels := e.getPodMetadata(endpoint, matchLabels)

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
			Replicas: endpoint.Spec.WebSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: podAnnotations,
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
	if endpoint.Spec.WebSpec.Image != nil {
		return *endpoint.Spec.WebSpec.Image
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
	}

	container.Command = []string{}

	container.Resources = endpoint.Spec.WebSpec.Resources
	container.Env = []corev1.EnvVar{
		{
			Name:  "OPENAI_API_KEY",
			Value: *endpoint.Spec.WebSpec.OpenAiKey,
		},
		{
			Name:  "CODE",
			Value: *endpoint.Spec.WebSpec.Password,
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
	servicePort := corev1.ServicePort{
		Name:       endpoint.Spec.WebSpec.PortName,
		Port:       aimanageriov1alpha1.WebServicePort,
		TargetPort: intstr.FromString(endpoint.Spec.WebSpec.PortName),
		Protocol:   corev1.ProtocolTCP,
	}

	svc = &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", aimanageriov1alpha1.ChatWebName, endpoint.Name),
			Namespace: endpoint.Namespace,
			Labels: map[string]string{
				aimanageriov1alpha1.LabelApp: endpoint.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
		},
	}

	if endpoint.Spec.WebSpec.NodePort != nil {
		servicePort.NodePort = *endpoint.Spec.WebSpec.NodePort
		svc.Spec.Type = corev1.ServiceTypeNodePort
		svc.Spec.Ports = []corev1.ServicePort{servicePort}
	}
	return
}

func (e *EndpointReconciler) CreateOrUpdateService(ctx context.Context, desired *corev1.Service) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current = &corev1.Service{}
		err := e.Get(ctx, client.ObjectKeyFromObject(desired), current)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return e.Create(ctx, desired)
		}

		// Apply immutable fields from the existing service.
		desired.Spec.IPFamilies = current.Spec.IPFamilies
		desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		desired.Spec.ClusterIP = current.Spec.ClusterIP
		desired.Spec.ClusterIPs = current.Spec.ClusterIPs

		mergeMetadata(&desired.ObjectMeta, &current.ObjectMeta)

		return e.Update(ctx, desired)
	})
}

func (e *EndpointReconciler) UpdateStatus(ctx context.Context, endpoint *aimanageriov1alpha1.Endpoint) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		currEndpoint := &aimanageriov1alpha1.Endpoint{}
		err := e.Get(ctx, client.ObjectKeyFromObject(endpoint), currEndpoint)
		if err != nil {
			return err
		}

		currEndpoint.Status = endpoint.Status
		return e.Status().Update(ctx, currEndpoint)
	})
}

func (e *EndpointReconciler) CreateOrUpdateInference(ctx context.Context, desiredInference *aimanageriov1alpha1.Inference) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var currInference = &aimanageriov1alpha1.Inference{}
		err := e.Get(ctx, client.ObjectKeyFromObject(desiredInference), currInference)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return e.Create(ctx, desiredInference)
		}

		mergeMetadata(&desiredInference.ObjectMeta, &currInference.ObjectMeta)
		return e.Update(ctx, desiredInference)
	})
}
