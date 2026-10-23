package reconciler

import (
	"context"
	"slices"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	createdResourcesLogger      = ctrl.Log.WithName("created_resources")
	createdResourcesLoggerDebug = ctrl.Log.WithName("created_resources").V((1))
)

func (r *Reconciler) ReconcileMRCreatedResources(ctx context.Context, managedResource *automationv1alpha1.ManagedResource, namespaces []corev1.Namespace) error {
	matching := map[string]bool{}
	for i := range namespaces {
		matches, err := namespaceMatchesManagedResource(&namespaces[i], managedResource)
		if err != nil {
			reconcilerLogger.Error(err, "Error matching namespace with managed resource")
			return err
		}
		if matches {
			matching[namespaces[i].Name] = true
		}
	}

	// Iterate over a copy: removeResource shifts the remaining entries of the slice in place.
	for _, createdResource := range slices.Clone(managedResource.Status.CreatedResources) {
		// The namespace pass recreates missing resources of matching namespaces.
		if matching[createdResource.TriggerNamespace] {
			continue
		}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(createdResource.ApiVersion)
		obj.SetKind(createdResource.Kind)
		obj.SetName(createdResource.Name)
		obj.SetNamespace(createdResource.Namespace)
		ri, err := kube.GetResourceInterfaceForUnstructured(obj, r.RESTMapper, r.DynamicClient)
		if err != nil {
			return err
		}

		createdResourcesLogger.Info("Trigger namespace no longer exists or matches, removing resource", "triggerNamespace", createdResource.TriggerNamespace, "resource", createdResource)
		err = ri.Delete(ctx, createdResource.Name, metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
			createdResourcesLoggerDebug.Info("Resource already deleted", "resource", createdResource)
		}
		managedResource.Status.CreatedResources = removeResource(managedResource.Status.CreatedResources, createdResource)
		createdResourcesLogger.Info("Resource removed", "resource", createdResource)
	}
	return nil
}
