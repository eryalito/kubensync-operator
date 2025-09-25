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
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"github.com/eryalito/kubensync-operator/internal/reconciler"
	corev1 "k8s.io/api/core/v1"
)

// ManagedResourceReconciler reconciles a ManagedResource object
type ManagedResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
		// Set Ready condition to False with reason Error
		setCondition(&mr.Status, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "ReconcileError",
			Message:            err.Error(),
			ObservedGeneration: mr.GetGeneration(),
		})
		// try to update status ignoring further errors, but return original err
		_ = r.Status().Update(ctx, mr)
		return ctrl.Result{}, err
	}
	setCondition(&mr.Status, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "Reconciled",
		Message:            "ManagedResource successfully reconciled",
		ObservedGeneration: mr.GetGeneration(),
	})
	if updateErr := r.Status().Update(ctx, mr); updateErr != nil {
		return ctrl.Result{}, updateErr
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

	for _, nsDef := range nsList.Items {
		MRDef, err := kube.GetManagedResource(ctx, managedresource.GetName())
		if err != nil {
			return err
		}
		originalMRDef := MRDef.DeepCopy()
		MRDef, err = rdr.ReconcileNamespaceChange(ctx, MRDef, &nsDef)
		if err != nil {
			return err
		}
		if kube.AreManagedResourcesStatusDifferent(originalMRDef.Status, MRDef.Status) {
			managedResourceController.Info("Updating status", "name", MRDef.Name)
			err = kube.UpdateStatus(MRDef, ctx)
			if err != nil {
				return err
			}
		}
	}

	// Process existing resources in status field
	loadedManagedResource, err := kube.GetManagedResource(ctx, managedresource.GetName())
	if err != nil {
		return err
	}
	err = rdr.ReconcileMRCreatedResources(ctx, loadedManagedResource)
	if err != nil {
		return err
	}

	return nil

}

// setCondition adds or updates a condition in the status slice
func setCondition(status *automationv1alpha1.ManagedResourceStatus, condition metav1.Condition) {
	// remove existing of same type
	newConds := []metav1.Condition{}
	for _, c := range status.Conditions {
		if c.Type == condition.Type {
			continue
		}
		newConds = append(newConds, c)
	}
	newConds = append(newConds, condition)
	status.Conditions = newConds
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&automationv1alpha1.ManagedResource{}).
		// Watch Namespaces and enqueue ManagedResources that match the namespace selector
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.mapNamespaceToManagedResources)).
		Named("managedresource").
		Complete(r)
}

// mapNamespaceToManagedResources lists ManagedResources and returns reconcile requests for those whose selector matches the namespace.
func (r *ManagedResourceReconciler) mapNamespaceToManagedResources(ctx context.Context, obj client.Object) []reconcile.Request {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		return nil
	}
	// List all ManagedResources (optimize later with index if needed)
	mrList := &automationv1alpha1.ManagedResourceList{}
	if err := r.List(ctx, mrList); err != nil {
		managedResourceController.Error(err, "listing ManagedResources for namespace event", "namespace", ns.Name)
		return nil
	}
	var reqs []reconcile.Request
	for _, mr := range mrList.Items {
		if namespaceMatchesMR(ns, &mr) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKey{Name: mr.Name}})
		}
	}
	return reqs
}

// mapNamespaceToManagedResources lists MRs and enqueues those whose selector matches this namespace.
// namespaceMatchesMR replicates the selector logic quickly (regex + label selector if defined).
func namespaceMatchesMR(ns *corev1.Namespace, mr *automationv1alpha1.ManagedResource) bool {
	sel := mr.Spec.NamespaceSelector
	// Label selector check
	if len(sel.LabelSelector.MatchLabels) > 0 || len(sel.LabelSelector.MatchExpressions) > 0 {
		ls, err := metav1.LabelSelectorAsSelector(&sel.LabelSelector)
		if err != nil || !ls.Matches(labels.Set(ns.Labels)) {
			return false
		}
	}
	if sel.Regex != "" {
		matched, err := regexp.MatchString(sel.Regex, ns.Name)
		if err != nil || !matched {
			return false
		}
	}
	return true
}
