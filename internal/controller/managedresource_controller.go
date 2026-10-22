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
	"fmt"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
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

// statusCheckpointInterval bounds the tracking a reconcile loses if it stops mid-pass,
// without writing the whole status once per namespace.
var statusCheckpointInterval = 10 * time.Second

const statusWriteTimeout = 10 * time.Second

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
	reconciler.Mutex.Lock()
	defer reconciler.Mutex.Unlock()
	// Read from the API server once the lock is held: the namespace controller may have
	// written this status while we waited, and a cached copy would make the update conflict.
	mr := &automationv1alpha1.ManagedResource{}
	err := r.apiReader.Get(ctx, req.NamespacedName, mr)
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

	savedStatus := managedresource.Status.DeepCopy()
	saveStatus := func(mr *automationv1alpha1.ManagedResource) error {
		if !kube.AreManagedResourcesStatusDifferent(*savedStatus, mr.Status) {
			return nil
		}
		managedResourceController.Info("Updating status", "name", mr.Name)
		// Detached from the reconcile context so a shutdown mid-pass still records what it
		// created; the timeout only bounds writes made after cancellation.
		writeCtx := context.WithoutCancel(ctx)
		if ctx.Err() != nil {
			var cancel context.CancelFunc
			writeCtx, cancel = context.WithTimeout(writeCtx, statusWriteTimeout)
			defer cancel()
		}
		// Write a copy: the client decodes the server object into it, and after a retry that
		// object can carry a newer spec than the one this pass is rendering.
		toSave := mr.DeepCopy()
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			updateErr := r.Status().Update(writeCtx, toSave)
			if !errors.IsConflict(updateErr) {
				return updateErr
			}
			fresh := &automationv1alpha1.ManagedResource{}
			if err := r.apiReader.Get(writeCtx, client.ObjectKeyFromObject(mr), fresh); err != nil {
				return err
			}
			// Only write over a spec or metadata edit: status from another writer, or an
			// object that was recreated or is being deleted, must not be overwritten.
			changed := fresh.UID != mr.UID || fresh.DeletionTimestamp != nil ||
				kube.AreManagedResourcesStatusDifferent(*savedStatus, fresh.Status)
			if changed {
				return fmt.Errorf("ManagedResource %q changed during the reconcile", mr.Name)
			}
			toSave.ResourceVersion = fresh.ResourceVersion
			return updateErr
		})
		if err != nil {
			return err
		}
		mr.ResourceVersion = toSave.ResourceVersion
		savedStatus = toSave.Status.DeepCopy()
		return nil
	}

	currentMR := managedresource
	lastSave := time.Now()
	for _, nsDef := range nsList.Items {
		reconciledMR, reconcileErr := rdr.ReconcileNamespaceChange(ctx, currentMR, &nsDef)
		if reconciledMR != nil {
			currentMR = reconciledMR
		}
		if reconcileErr != nil {
			// Persist any resources created before the failure so the status keeps tracking them.
			_ = saveStatus(currentMR)
			return reconcileErr
		}
		if time.Since(lastSave) >= statusCheckpointInterval {
			if err = saveStatus(currentMR); err != nil {
				return err
			}
			lastSave = time.Now()
		}
	}
	// Processing tracked resources can take longer than the namespace pass, so save the
	// pass first rather than risk losing its tracking if the reconcile stops there.
	if err = saveStatus(currentMR); err != nil {
		return err
	}

	// Process existing resources in status field
	err = rdr.ReconcileMRCreatedResources(ctx, currentMR)
	if err != nil {
		_ = saveStatus(currentMR)
		return err
	}

	apimeta.SetStatusCondition(&currentMR.Status.Conditions, metav1.Condition{
		Type:               automationv1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            "All resources synced successfully",
		ObservedGeneration: currentMR.Generation,
	})
	return saveStatus(currentMR)
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
