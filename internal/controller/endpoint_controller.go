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
	"github.com/MDZZ110/ai-manager/internal/utils"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aimanageriov1alpha1 "github.com/MDZZ110/ai-manager/api/v1alpha1"
)

// EndpointReconciler reconciles a Endpoint object
type EndpointReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
	baseCtx := context.Background()

	if err := e.Get(baseCtx, req.NamespacedName, endpoint); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if endpoint.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsFinalizer(endpoint.GetFinalizers(), aimanageriov1alpha1.EndpointFinalizer) {
			e.SetFinalizers(endpoint)
			if err := e.Update(baseCtx, endpoint); err != nil {
				logger.V(0).Info("unable to add finalizer on endpoint resource", "name", endpoint.Name, "err", err.Error())
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		logger.V(0).Info("receive deletion request", "name", endpoint.Name)
		if utils.ContainsFinalizer(endpoint.GetFinalizers(), aimanageriov1alpha1.EndpointFinalizer) {
			err := e.ReleaseResources(baseCtx, req.NamespacedName, logger)
			if err != nil {
				return ctrl.Result{}, err
			}

			e.RemoveFinalizers(endpoint)
			if err := e.Update(baseCtx, endpoint); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
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
