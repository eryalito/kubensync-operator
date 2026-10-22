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
	apiReader client.Reader
	config    *rest.Config
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
	err = r.reconcileManagedResource(ctx, mr)
	if err != nil {
		freshMR := &automationv1alpha1.ManagedResource{}
		if fetchErr := r.apiReader.Get(ctx, req.NamespacedName, freshMR); fetchErr == nil {
			apimeta.SetStatusCondition(&freshMR.Status.Conditions, metav1.Condition{
				Type:               automationv1alpha1.ConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ReconcileError",
				Message:            err.Error(),
				ObservedGeneration: freshMR.Generation,
			})
			_ = r.Status().Update(ctx, freshMR)
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ManagedResourceReconciler) reconcileManagedResource(ctx context.Context, managedresource *automationv1alpha1.ManagedResource) error {
	var err error
	var nsList *corev1.NamespaceList
	rdr := reconciler.Reconciler{RestConfig: r.config}

	rdr.Clientset, err = kubernetes.NewForConfig(r.config)

	if err != nil {
		return err
	}

	reconciler.Mutex.Lock()
	defer reconciler.Mutex.Unlock()
	if managedresource.Spec.NamespaceSelector.LabelSelector.MatchLabels == nil {
		nsList, err = kube.GetNamespaces(ctx, r.config)
		if err != nil {
			return err
		}
	} else {
		nsList, err = kube.GetNamespacesByLabel(ctx, r.config, managedresource.Spec.NamespaceSelector.LabelSelector)
		if err != nil {
			return err
		}
	}

	originalStatus := managedresource.Status.DeepCopy()
	currentMR := managedresource
	for _, nsDef := range nsList.Items {
		reconciledMR, reconcileErr := rdr.ReconcileNamespaceChange(ctx, currentMR, &nsDef)
		if reconcileErr != nil {
			// Persist any resources created before the failure so the status keeps tracking them.
			if reconciledMR != nil && kube.AreManagedResourcesStatusDifferent(*originalStatus, reconciledMR.Status) {
				_ = r.Status().Update(ctx, reconciledMR)
			}
			return reconcileErr
		}
		currentMR = reconciledMR
	}

	// Process existing resources in status field
	err = rdr.ReconcileMRCreatedResources(ctx, currentMR)
	if err != nil {
		if kube.AreManagedResourcesStatusDifferent(*originalStatus, currentMR.Status) {
			_ = r.Status().Update(ctx, currentMR)
		}
		return err
	}

	apimeta.SetStatusCondition(&currentMR.Status.Conditions, metav1.Condition{
		Type:               automationv1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            "All resources synced successfully",
		ObservedGeneration: currentMR.Generation,
	})
	if !kube.AreManagedResourcesStatusDifferent(*originalStatus, currentMR.Status) {
		return nil
	}
	managedResourceController.Info("Updating status", "name", currentMR.Name)
	return r.Status().Update(ctx, currentMR)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	r.apiReader = mgr.GetAPIReader()
	return ctrl.NewControllerManagedBy(mgr).
		For(&automationv1alpha1.ManagedResource{}).
		Named("managedresource").
		Complete(r)
}
