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
	stderrors "errors"
	"fmt"
	"slices"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"github.com/eryalito/kubensync-operator/internal/reconciler"
	"github.com/eryalito/kubensync-operator/internal/watch"
	corev1 "k8s.io/api/core/v1"
)

// ManagedResourceReconciler reconciles a ManagedResource object
type ManagedResourceReconciler struct {
	client.Client
	apiReader       client.Reader
	config          *rest.Config
	discoveryClient discovery.DiscoveryInterface
	dynamicClient   dynamic.Interface
	watches         *watch.Manager
}

var managedResourceController = ctrl.Log.WithName("managedresource_controller")

// statusCheckpointInterval bounds the tracking a reconcile loses if it stops mid-pass,
// without writing the whole status once per namespace.
var statusCheckpointInterval = 10 * time.Second

const statusWriteTimeout = 10 * time.Second

// dataWatchDebounce delays the reconcile caused by a change to watched data, so changes made within it of the
// first one are merged into that reconcile.
const dataWatchDebounce = 500 * time.Millisecond

const dataWatchSyncTimeout = 10 * time.Second

// resyncPredicate passes the cache's periodic resync events, which GenerationChangedPredicate drops.
var resyncPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion()
	},
}

// readyBecameFalsePredicate passes the status write that marks the ManagedResource not ready. Only this
// controller sets Ready back to true, so a failure written by the namespace controller must reach it.
var readyBecameFalsePredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldMR, okOld := e.ObjectOld.(*automationv1alpha1.ManagedResource)
		newMR, okNew := e.ObjectNew.(*automationv1alpha1.ManagedResource)
		return okOld && okNew &&
			!apimeta.IsStatusConditionFalse(oldMR.Status.Conditions, automationv1alpha1.ConditionReady) &&
			apimeta.IsStatusConditionFalse(newMR.Status.Conditions, automationv1alpha1.ConditionReady)
	},
}

// managedResourceEvents ignores status-only updates, so the controller's own status writes do not
// trigger another reconcile.
var managedResourceEvents = predicate.Or(
	predicate.GenerationChangedPredicate{},
	predicate.AnnotationChangedPredicate{},
	predicate.LabelChangedPredicate{},
	resyncPredicate,
	readyBecameFalsePredicate,
)

// +kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources/finalizers,verbs=update

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
			r.watches.Remove(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	} else if mr.DeletionTimestamp != nil {
		r.watches.Remove(req.Name)
		return ctrl.Result{}, nil
	}
	mapper := kube.NewRESTMapper(r.discoveryClient)
	// Watch before rendering, so creating a missing data resource triggers the reconcile that fixes it.
	keys, err := watch.Keys(mr.Spec.Template.Data, mapper)
	if err != nil {
		managedResourceController.Error(err, "Error resolving watched data", "name", mr.Name)
	}
	if err := r.watches.Sync(ctx, mr.Name, keys); err != nil {
		managedResourceController.Error(err, "Error watching data", "name", mr.Name)
	}
	err = r.reconcileManagedResource(ctx, mr, mapper)
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

func (r *ManagedResourceReconciler) reconcileManagedResource(ctx context.Context, managedresource *automationv1alpha1.ManagedResource, mapper apimeta.RESTMapper) error {
	var err error
	var nsList *corev1.NamespaceList
	rdr := reconciler.Reconciler{RESTMapper: mapper, DynamicClient: r.dynamicClient}

	rdr.Clientset, err = kubernetes.NewForConfig(r.config)

	if err != nil {
		return err
	}

	nsList, err = r.listNamespaces(ctx, managedresource)
	if err != nil {
		return err
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
	// A failing namespace must not keep the others, or the cleanup below, from being reconciled.
	var namespaceErrs []error
	for _, nsDef := range nsList.Items {
		reconciledMR, reconcileErr := rdr.ReconcileNamespaceChange(ctx, currentMR, &nsDef)
		if reconciledMR != nil {
			currentMR = reconciledMR
		}
		if reconcileErr != nil {
			namespaceErrs = append(namespaceErrs, fmt.Errorf("namespace %s: %w", nsDef.Name, reconcileErr))
			continue
		}
		if time.Since(lastSave) >= statusCheckpointInterval {
			if err = saveStatus(currentMR); err != nil {
				return err
			}
			lastSave = time.Now()
		}
	}
	// Save the pass before removing stale resources, so its tracking survives if the reconcile stops there.
	if err = saveStatus(currentMR); err != nil {
		return err
	}

	// Listed again so namespaces deleted or relabelled during the pass lose their resources in this reconcile.
	nsList, err = r.listNamespaces(ctx, currentMR)
	if err != nil {
		return err
	}
	err = rdr.ReconcileMRCreatedResources(ctx, currentMR, nsList.Items)
	if err != nil {
		_ = saveStatus(currentMR)
		return err
	}
	if len(namespaceErrs) > 0 {
		if err = saveStatus(currentMR); err != nil {
			return err
		}
		return joinErrors(namespaceErrs)
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

// maxReportedErrors bounds how many errors end up in the Ready condition message.
const maxReportedErrors = 5

func joinErrors(errs []error) error {
	if len(errs) <= maxReportedErrors {
		return stderrors.Join(errs...)
	}
	reported := append(slices.Clone(errs[:maxReportedErrors]), fmt.Errorf("and %d more", len(errs)-maxReportedErrors))
	return stderrors.Join(reported...)
}

func (r *ManagedResourceReconciler) listNamespaces(ctx context.Context, mr *automationv1alpha1.ManagedResource) (*corev1.NamespaceList, error) {
	if mr.Spec.NamespaceSelector.LabelSelector.MatchLabels == nil {
		return kube.GetNamespaces(ctx, r.config)
	}
	return kube.GetNamespacesByLabel(ctx, r.config, mr.Spec.NamespaceSelector.LabelSelector)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	r.apiReader = mgr.GetAPIReader()
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.discoveryClient = discoveryClient
	dynamicClient, err := dynamic.NewForConfigAndClient(mgr.GetConfig(), mgr.GetHTTPClient())
	if err != nil {
		return err
	}
	r.dynamicClient = dynamicClient
	// Watched objects are small, so compressing their watch streams only costs each one a gzip reader.
	watchConfig := rest.CopyConfig(mgr.GetConfig())
	watchConfig.DisableCompression = true
	metadataClient, err := metadata.NewForConfig(watchConfig)
	if err != nil {
		return err
	}
	r.watches = watch.NewManager(watch.MetadataInformers(metadataClient), dataWatchSyncTimeout)
	if err := mgr.Add(r.watches); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&automationv1alpha1.ManagedResource{}, builder.WithPredicates(managedResourceEvents)).
		WatchesRawSource(source.Channel(r.watches.Events(), handler.TypedFuncs[string, reconcile.Request]{
			GenericFunc: func(_ context.Context, e event.TypedGenericEvent[string], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				q.AddAfter(reconcile.Request{NamespacedName: types.NamespacedName{Name: e.Object}}, dataWatchDebounce)
			},
		})).
		Named("managedresource").
		Complete(r)
}
