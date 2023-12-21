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
	aimanageriov1alpha1 "github.com/MDZZ110/ai-manager/api/v1alpha1"
	"github.com/MDZZ110/ai-manager/internal/config"
	"github.com/MDZZ110/ai-manager/internal/utils"
	"github.com/go-logr/logr"
	"github.com/mitchellh/hashstructure"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"strings"
	"time"
)

// InferenceReconciler reconciles a Inference object
type InferenceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *config.Config
}

//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (i *InferenceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	logger := log.FromContext(ctx)
	inference := &aimanageriov1alpha1.Inference{}

	if err := i.Get(ctx, req.NamespacedName, inference); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	inference = i.SetDefaultValue(inference)
	if inference.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsFinalizer(inference.GetFinalizers(), aimanageriov1alpha1.InferenceFinalizer) {
			i.SetFinalizers(inference)
			if err := i.Update(ctx, inference); err != nil {
				logger.V(0).Info("unable to add finalizer on inference resource", "name", inference.Name, "err", err.Error())
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		logger.V(0).Info("receive deletion request", "name", inference.Name)
		if utils.ContainsFinalizer(inference.GetFinalizers(), aimanageriov1alpha1.InferenceFinalizer) {
			err := i.ReleaseResources(ctx, req.NamespacedName, logger)
			if err != nil {
				return ctrl.Result{}, err
			}

			i.RemoveFinalizers(inference)
			err = i.Update(ctx, inference)
			if err != nil {
				logger.Error(err, "update inference failed", "inferName", inference.Name)
			}

			return ctrl.Result{}, err
		}
	}

	matchLabels := map[string]string{
		aimanageriov1alpha1.LabelApp:       inference.Name,
		aimanageriov1alpha1.LabelModelName: utils.ReplaceModelName(inference.Spec.Model),
		aimanageriov1alpha1.LabelFramework: inference.Spec.Framework,
	}

	hashInt64, err := hashstructure.Hash(struct {
		Name            string
		InferGeneration int64
		Config          config.Config
	}{
		Name:            inference.Name,
		InferGeneration: inference.Generation,
		Config:          *i.Config,
	}, nil)
	if err != nil {
		logger.Error(err, "failed to calculate combined inference hash", "inferName", inference.Name)
		return ctrl.Result{}, err
	}

	hash := fmt.Sprintf("%d", hashInt64)
	if inference.Status.Hash == hash {
		logger.V(0).Info("combinedHash not updated, skipping updating inference component",
			"oldHash", inference.Status.Hash,
			"newHash", hash,
			"inferName", inference.Name)
		return ctrl.Result{}, nil
	}

	defer func() {
		inference.Status.Hash = hash
		inference.Status.LastTransitionTime = metav1.Time{
			Time: time.Now().UTC(),
		}

		if err != nil {
			inference.Status.Message = err.Error()
			inference.Status.Status = aimanageriov1alpha1.DeployedInferStatus
		} else {
			inference.Status.Status = aimanageriov1alpha1.FailedInferStatus
		}

		err = i.UpdateStatus(ctx, inference)
		if err != nil {
			logger.Error(err, "update inference status failed", "inferName", inference.Name)
		}
	}()

	desiredDeployment := i.getDesiredWorkerDeployment(inference, matchLabels)
	err = ctrl.SetControllerReference(inference, desiredDeployment, i.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = i.CreateOrUpdateDeployment(ctx, desiredDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	desiredService := getDesiredService(inference, matchLabels)
	err = ctrl.SetControllerReference(inference, desiredService, i.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = i.CreateOrUpdateService(ctx, desiredService)
	if err != nil {
		logger.Error(err, "create or update service failed", "inferName", inference.Name)
	}
	return ctrl.Result{}, err
}

func (i *InferenceReconciler) SetDefaultValue(inference *aimanageriov1alpha1.Inference) *aimanageriov1alpha1.Inference {
	if len(inference.Spec.ImagePullSecrets) == 0 {
		credSecretRef := corev1.LocalObjectReference{i.Config.CredSecretName}
		inference.Spec.ImagePullSecrets = append(inference.Spec.ImagePullSecrets, credSecretRef)
	}

	return inference
}

func (i *InferenceReconciler) UpdateStatus(ctx context.Context, inference *aimanageriov1alpha1.Inference) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		currInference := &aimanageriov1alpha1.Inference{}
		err := i.Get(ctx, client.ObjectKeyFromObject(inference), currInference)
		if err != nil {
			return err
		}

		currInference.Status = currInference.Status
		return i.Status().Update(ctx, currInference)
	})
}

func (i *InferenceReconciler) CreateOrUpdateDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current = &appsv1.Deployment{}
		err := i.Get(ctx, client.ObjectKeyFromObject(desired), current)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return i.Create(ctx, desired)
		}

		mergeMetadata(&desired.ObjectMeta, &current.ObjectMeta)
		return i.Update(ctx, desired)
	})
}

func (i *InferenceReconciler) CreateOrUpdateService(ctx context.Context, desired *corev1.Service) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current = &corev1.Service{}
		err := i.Get(ctx, client.ObjectKeyFromObject(desired), current)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			return i.Create(ctx, desired)
		}

		// Apply immutable fields from the existing service.
		desired.Spec.IPFamilies = current.Spec.IPFamilies
		desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		desired.Spec.ClusterIP = current.Spec.ClusterIP
		desired.Spec.ClusterIPs = current.Spec.ClusterIPs

		mergeMetadata(&desired.ObjectMeta, &current.ObjectMeta)

		return i.Update(ctx, desired)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (i *InferenceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aimanageriov1alpha1.Inference{}).
		Complete(i)
}

func (i *InferenceReconciler) SetFinalizers(inference *aimanageriov1alpha1.Inference) {
	finalizers := inference.GetFinalizers()
	finalizers = append(finalizers, aimanageriov1alpha1.InferenceFinalizer)
	inference.SetFinalizers(finalizers)
}

func (i *InferenceReconciler) RemoveFinalizers(inference *aimanageriov1alpha1.Inference) {
	finalizers := inference.GetFinalizers()
	for i := 0; i < len(finalizers); i++ {
		if finalizers[i] == aimanageriov1alpha1.InferenceFinalizer {
			newFinalizers := append(finalizers[:i], finalizers[i+1:]...)
			inference.SetFinalizers(newFinalizers)
			return
		}
	}
}

// TODO release
func (i *InferenceReconciler) ReleaseResources(ctx context.Context, nn types.NamespacedName, log logr.Logger) (err error) {
	return
}

func mergeMetadata(new, old *metav1.ObjectMeta) {
	new.ResourceVersion = old.ResourceVersion
	new.Labels = labels.Merge(old.Labels, new.Labels)
	new.Annotations = labels.Merge(old.Annotations, new.Annotations)
}

func (i *InferenceReconciler) getPodMetadata(inference *aimanageriov1alpha1.Inference, matchLabels map[string]string) (map[string]string, map[string]string) {
	podAnnotations := make(map[string]string)
	podLabels := make(map[string]string)
	for k, v := range matchLabels {
		podLabels[k] = v
	}

	if inference.Spec.PodMetadata != nil {
		for k, v := range inference.Spec.PodMetadata.Labels {
			podLabels[k] = v
		}

		for k, v := range inference.Spec.PodMetadata.Annotations {
			podAnnotations[k] = v
		}
	}

	return podAnnotations, podLabels
}

func (i *InferenceReconciler) getDesiredWorkerDeployment(inference *aimanageriov1alpha1.Inference, matchLabels map[string]string) (workerDeployment *appsv1.Deployment) {
	container := i.createContainer(inference)
	podAnnotations, podLabels := i.getPodMetadata(inference, matchLabels)

	workerDeployment = &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", inference.Name, utils.ReplaceModelName(inference.Spec.Model), inference.Spec.Framework),
			Namespace: inference.Namespace,
			Labels:    matchLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: inference.Spec.Replicas,
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
					NodeSelector:    inference.Spec.NodeSelector,
					Tolerations:     inference.Spec.Tolerations,
					Affinity:        inference.Spec.Affinity,
				},
			},
		},
	}

	workerDeployment.Spec.Template.Spec.Containers = []corev1.Container{container}
	return
}

func (i *InferenceReconciler) getModelImage(inference *aimanageriov1alpha1.Inference) (modelImage string) {
	if inference.Spec.Image != nil {
		return *inference.Spec.Image
	}

	modelName := utils.ReplaceModelName(inference.Spec.Model)
	if strings.HasSuffix(i.Config.Registry, "/") {
		return fmt.Sprintf("%s%s-%s:%s", i.Config.Registry, modelName, inference.Spec.Framework, aimanageriov1alpha1.DefaultModelImageTag)
	}

	return fmt.Sprintf("%s/%s-%s:%s", i.Config.Registry, modelName, inference.Spec.Framework, aimanageriov1alpha1.DefaultModelImageTag)
}

func (i *InferenceReconciler) createContainer(inference *aimanageriov1alpha1.Inference) (container corev1.Container) {
	container = corev1.Container{
		Name:            inference.Name,
		Image:           i.getModelImage(inference),
		ImagePullPolicy: inference.Spec.ImagePullPolicy,
		Ports:           createInferServicePorts(inference),
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

	container.Command = []string{
		"python",
		"-m",
		"vllm.entrypoints.openai.api_server",
		"--host",
		"0.0.0.0",
		"--port",
		strconv.Itoa(int(aimanageriov1alpha1.InferContainerPort)),
		"--model",
		fmt.Sprintf("%s/%s", aimanageriov1alpha1.LocalModelDir, inference.Spec.Model),
	}

	container.Resources = inference.Spec.Resources
	return
}

func createInferServicePorts(inference *aimanageriov1alpha1.Inference) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          inference.Spec.PortName,
			ContainerPort: aimanageriov1alpha1.InferContainerPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
}

func getDesiredService(inference *aimanageriov1alpha1.Inference, labels map[string]string) (svc *corev1.Service) {
	servicePort := corev1.ServicePort{
		Name:       inference.Spec.PortName,
		Port:       aimanageriov1alpha1.InferServicePort,
		TargetPort: intstr.FromString(inference.Spec.PortName),
		Protocol:   corev1.ProtocolTCP,
	}

	svc = &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      inference.Name,
			Namespace: inference.Namespace,
			Labels: map[string]string{
				aimanageriov1alpha1.LabelApp: inference.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
		},
	}

	if inference.Spec.NodePort != nil {
		servicePort.NodePort = *inference.Spec.NodePort
		svc.Spec.Type = corev1.ServiceTypeNodePort
		svc.Spec.Ports = []corev1.ServicePort{servicePort}
	}

	return
}
