package controller

import (
	// Import necessary packages

	"context"
	"log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"github.com/eryalito/kubensync-operator/internal/reconciler"
)

// NamespaceController reconciles Custom Resources and responds to namespace events.
type NamespaceController struct {
	client.Client
	Scheme *runtime.Scheme
	config *rest.Config
}

var namespaceControllerLogger = ctrl.Log.WithName("namespace_controller")

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

func (r *NamespaceController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Handle the namespace event here
	namespaceControllerLogger.Info("Reconciling Namespace", "name", req.Name)
	ns := &corev1.Namespace{}
	err := r.Get(ctx, req.NamespacedName, ns)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, which means it was deleted
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	} else if ns.DeletionTimestamp != nil {
		// Object is being deleted
		return reconcile.Result{}, nil
	}
	err = reconcileNamespace(ctx, r.config, ns)
	if err != nil {
		return ctrl.Result{}, err
	}
	return reconcile.Result{}, nil
}

// Watch for Namespace events.
func (r *NamespaceController) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}

func reconcileNamespace(ctx context.Context, config *rest.Config, namespace *corev1.Namespace) error {
	var err error
	var mrDefList automationv1alpha1.ManagedResourceList
	rdr := reconciler.Reconciler{RestConfig: config}

	rdr.Clientset, err = kubernetes.NewForConfig(config)

	if err != nil {
		return err
	}

	reconciler.Mutex.Lock()
	defer reconciler.Mutex.Unlock()
	mrDefList, err = kube.GetManagedResources(ctx)
	if err != nil {
		return err
	}

	for _, mrDef := range mrDefList.Items {
		originalMRDef := mrDef.DeepCopy()
		newMRDef, err := rdr.ReconcileNamespaceChange(ctx, &mrDef, namespace)
		if err != nil {
			return err
		}
		if kube.AreManagedResourcesStatusDifferent(originalMRDef.Status, newMRDef.Status) {
			log.Printf("Updating status for %s", newMRDef.Name)
			err = kube.UpdateStatus(newMRDef, ctx)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
