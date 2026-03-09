/*
Copyright 2025.

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

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"github.com/eryalito/kubensync-operator/internal/reconciler"
	corev1 "k8s.io/api/core/v1"
)

// ManagedResourceReconciler reconciles a ManagedResource object
type ManagedResourceReconciler struct {
	client.Client
	config *rest.Config
}

var managedResourceController = ctrl.Log.WithName("managedresource_controller")

// +kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ManagedResource object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *ManagedResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	managedResourceController.Info("Reconciling ManagedResource", "name", req.Name)
	mr := &automationv1alpha1.ManagedResource{}
	err := r.Get(ctx, req.NamespacedName, mr)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, which means it was deleted
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	} else if mr.DeletionTimestamp != nil {
		// Object is being deleted
		return ctrl.Result{}, nil
	}
	err = reconcileManagedResource(ctx, r.config, mr)
	if err != nil {
		freshMR, fetchErr := kube.GetManagedResource(ctx, mr.GetName())
		if fetchErr == nil {
			apimeta.SetStatusCondition(&freshMR.Status.Conditions, metav1.Condition{
				Type:               automationv1alpha1.ConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ReconcileError",
				Message:            err.Error(),
				ObservedGeneration: freshMR.Generation,
			})
			_ = kube.UpdateStatus(freshMR, ctx)
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func reconcileManagedResource(ctx context.Context, config *rest.Config, managedresource *automationv1alpha1.ManagedResource) error {
	var err error
	var nsList *corev1.NamespaceList
	rdr := reconciler.Reconciler{RestConfig: config}

	rdr.Clientset, err = kubernetes.NewForConfig(config)

	if err != nil {
		return err
	}

	reconciler.Mutex.Lock()
	defer reconciler.Mutex.Unlock()
	if managedresource.Spec.NamespaceSelector.LabelSelector.MatchLabels == nil {
		nsList, err = kube.GetNamespaces(ctx, config)
		if err != nil {
			return err
		}
	} else {
		nsList, err = kube.GetNamespacesByLabel(ctx, config, managedresource.Spec.NamespaceSelector.LabelSelector)
		if err != nil {
			return err
		}
	}

	currentMR := managedresource
	for _, nsDef := range nsList.Items {
		originalMRDef := currentMR.DeepCopy()
		currentMR, err = rdr.ReconcileNamespaceChange(ctx, currentMR, &nsDef)
		if err != nil {
			return err
		}
		if kube.AreManagedResourcesStatusDifferent(originalMRDef.Status, currentMR.Status) {
			managedResourceController.Info("Updating status", "name", currentMR.Name)
			err = kube.UpdateStatus(currentMR, ctx)
			if err != nil {
				return err
			}
		}
	}

	// Process existing resources in status field
	err = rdr.ReconcileMRCreatedResources(ctx, currentMR)
	if err != nil {
		return err
	}

	apimeta.SetStatusCondition(&currentMR.Status.Conditions, metav1.Condition{
		Type:               automationv1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            "All resources synced successfully",
		ObservedGeneration: currentMR.Generation,
	})
	return kube.UpdateStatus(currentMR, ctx)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&automationv1alpha1.ManagedResource{}).
		Named("managedresource").
		Complete(r)
}
