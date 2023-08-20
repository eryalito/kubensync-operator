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

package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	automationv1alpha1 "github.com/kubensync/operator/api/v1alpha1"
	"github.com/kubensync/operator/pkg/kube"
	"github.com/kubensync/operator/pkg/reconciler"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// ManagedResourceReconciler reconciles a ManagedResource object
type ManagedResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	config *rest.Config
}

//+kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=automation.kubensync.com,resources=managedresources/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ManagedResource object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *ManagedResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infof("Reconciling ManagedResource %s", req.NamespacedName)
	mr := &automationv1alpha1.ManagedResource{}
	err := r.Get(ctx, req.NamespacedName, mr)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, which means it was deleted
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	err = reconcileManagedResource(ctx, r.config, mr)
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

	nsList, err = kube.GetNamespaces(ctx, config)
	if err != nil {
		return err
	}

	for _, nsDef := range nsList.Items {
		err = rdr.ReconcileNamespaceChange(ctx, managedresource, &nsDef)
		if err != nil {
			return err
		}
	}

	return nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&automationv1alpha1.ManagedResource{}).
		Complete(r)
}
