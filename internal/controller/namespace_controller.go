package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
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
	apiReader       client.Reader
	config          *rest.Config
	discoveryClient discovery.DiscoveryInterface
	dynamicClient   dynamic.Interface
}

var namespaceControllerLogger = ctrl.Log.WithName("namespace_controller")

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

func (r *NamespaceController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Handle the namespace event here
	namespaceControllerLogger.Info("Reconciling Namespace", "name", req.Name)
	ns := &corev1.Namespace{}
	err := r.Get(ctx, req.NamespacedName, ns)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		// Already deleted: handled as terminating, so the resources it triggered are removed.
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: req.Name, DeletionTimestamp: &metav1.Time{Time: time.Now()}}}
	}
	err = r.reconcileNamespace(ctx, ns)
	if err != nil {
		return ctrl.Result{}, err
	}
	return reconcile.Result{}, nil
}

// Watch for Namespace events.
func (r *NamespaceController) SetupWithManager(mgr ctrl.Manager) error {
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}

func (r *NamespaceController) reconcileNamespace(ctx context.Context, namespace *corev1.Namespace) error {
	var err error
	var mrDefList automationv1alpha1.ManagedResourceList
	rdr := reconciler.Reconciler{RESTMapper: kube.NewRESTMapper(r.discoveryClient), DynamicClient: r.dynamicClient}

	rdr.Clientset, err = kubernetes.NewForConfig(r.config)

	if err != nil {
		return err
	}

	reconciler.Mutex.Lock()
	defer reconciler.Mutex.Unlock()
	// Read from the API server: the cache can lag behind status the ManagedResource
	// controller just wrote, and updating status from a stale copy would conflict.
	err = r.apiReader.List(ctx, &mrDefList)
	if err != nil {
		return err
	}

	// A failing ManagedResource must not keep the others from handling this namespace.
	var errs []error
	for _, mrDef := range mrDefList.Items {
		originalMRDef := mrDef.DeepCopy()
		newMRDef, err := rdr.ReconcileNamespaceChange(ctx, &mrDef, namespace)
		if err != nil {
			// Prefer the returned resource so any partially applied resources stay tracked.
			statusMR := &mrDef
			if newMRDef != nil {
				statusMR = newMRDef
			}
			apimeta.SetStatusCondition(&statusMR.Status.Conditions, metav1.Condition{
				Type:               automationv1alpha1.ConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ReconcileError",
				Message:            err.Error(),
				ObservedGeneration: statusMR.Generation,
			})
			_ = r.Status().Update(ctx, statusMR)
			errs = append(errs, fmt.Errorf("ManagedResource %s: %w", mrDef.Name, err))
			continue
		}
		if kube.AreManagedResourcesStatusDifferent(originalMRDef.Status, newMRDef.Status) {
			namespaceControllerLogger.Info("Updating status", "name", newMRDef.Name)
			if err := r.Status().Update(ctx, newMRDef); err != nil {
				errs = append(errs, fmt.Errorf("ManagedResource %s: %w", mrDef.Name, err))
			}
		}
	}

	return joinErrors(errs)
}
