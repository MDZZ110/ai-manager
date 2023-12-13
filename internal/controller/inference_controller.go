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
	"github.com/MDZZ110/ai-manager/internal/utils"
	"github.com/go-logr/logr"
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

	aimanageriov1alpha1 "github.com/MDZZ110/ai-manager/api/v1alpha1"
)

// InferenceReconciler reconciles a Inference object
type InferenceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ai.manager.io,resources=inferences/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func (i *InferenceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	inference := &aimanageriov1alpha1.Inference{}

	if err := i.Get(ctx, req.NamespacedName, inference); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

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
			if err := i.Update(ctx, inference); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	matchLabels := map[string]string{
		aimanageriov1alpha1.LabelApp:       inference.Name,
		aimanageriov1alpha1.LabelModelName: inference.Spec.Model,
		aimanageriov1alpha1.LabelFramework: inference.Spec.Framework,
	}

	desiredDeployment := getDesiredWorkerDeployment(inference, matchLabels)
	desiredDeployment.SetOwnerReferences(GetOwnerReference(inference))
	err := i.CreateOrUpdateDeployment(ctx, desiredDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	desiredService := getDesiredService(inference, matchLabels)
	desiredService.SetOwnerReferences(GetOwnerReference(inference))
	err = i.CreateOrUpdateService(ctx, desiredService)
	return ctrl.Result{}, err
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

func GetOwnerReference(inference *aimanageriov1alpha1.Inference) []metav1.OwnerReference {
	isController := true
	controllerRef := []metav1.OwnerReference{
		{
			APIVersion: inference.APIVersion,
			Kind:       inference.Kind,
			Name:       inference.Name,
			UID:        inference.UID,
			Controller: &isController,
		},
	}

	return utils.MergeOwnerReferences(inference.GetOwnerReferences(), controllerRef)
}

func getDesiredWorkerDeployment(inference *aimanageriov1alpha1.Inference, matchLabels map[string]string) (workerDeployment *appsv1.Deployment) {
	container := createContainer(inference)
	workerDeployment = &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", inference.Name, inference.Spec.Model, inference.Spec.Framework),
			Namespace: inference.Namespace,
			Labels:    matchLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &inference.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: matchLabels,
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

func createContainer(inference *aimanageriov1alpha1.Inference) (container corev1.Container) {
	container = corev1.Container{
		Name:            inference.Name,
		Image:           inference.Spec.Image,
		ImagePullPolicy: inference.Spec.ImagePullPolicy,
		Ports:           createPrometheusPorts(),
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
	// TODO Hugging face Model container args
	//container.Args = []string{"-v=" + verbosity}

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

	container.Resources = inference.Spec.Resources
	return
}

func createPrometheusPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "infr-service",
			ContainerPort: 8082,
			Protocol:      "TCP",
		},
	}
}

func getDesiredService(inference *aimanageriov1alpha1.Inference, labels map[string]string) (svc *corev1.Service) {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: inference.Name,
			Labels: map[string]string{
				aimanageriov1alpha1.LabelApp: inference.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port: 8084,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 8084,
					},
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}
