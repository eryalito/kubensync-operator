package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
)

// NamespaceController reconciles Custom Resources and responds to namespace events.
type NamespaceController struct {
	client.Client
	Scheme *runtime.Scheme
	config *rest.Config
}

var namespaceControllerLogger = ctrl.Log.WithName("namespace_controller")

// Annotation used to nudge ManagedResource reconciliations when a Namespace event occurs
const NamespaceEventAnnotation = "kubensync.com/last-namespace-event"

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

// Reconcile patches matching ManagedResources with a timestamp annotation to trigger their own full aggregation reconcile.
func (r *NamespaceController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	mrList := &automationv1alpha1.ManagedResourceList{}
	if err := r.List(ctx, mrList); err != nil {
		return reconcile.Result{}, err
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	changed := 0
	for i := range mrList.Items {
		mr := &mrList.Items[i]
		if namespaceMatchesMR(ns, mr) {
			if mr.Annotations == nil {
				mr.Annotations = map[string]string{}
			}
			if mr.Annotations[NamespaceEventAnnotation] == stamp {
				continue
			}
			mr.Annotations[NamespaceEventAnnotation] = stamp
			if err := r.Update(ctx, mr); err != nil {
				namespaceControllerLogger.Error(err, "failed to patch ManagedResource for namespace event", "mr", mr.Name, "namespace", ns.Name)
				continue
			}
			changed++
		}
	}
	if changed > 0 {
		namespaceControllerLogger.Info("Triggered ManagedResource reconciles due to namespace event", "namespace", ns.Name, "managedResources", changed)
	}
	return reconcile.Result{}, nil
}

func (r *NamespaceController) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}
