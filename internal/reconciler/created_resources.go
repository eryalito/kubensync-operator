package reconciler

import (
	"context"
	"slices"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var createdResourcesLogger = ctrl.Log.WithName("created_resources")

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
		createdResourcesLogger.Info("Trigger namespace no longer exists or matches, removing resource", "triggerNamespace", createdResource.TriggerNamespace, "resource", createdResource)
		if !r.deleteTrackedResource(ctx, createdResource) {
			continue
		}
		managedResource.Status.CreatedResources = removeResource(managedResource.Status.CreatedResources, createdResource)
		createdResourcesLogger.Info("Resource removed", "resource", createdResource)
	}
	return nil
}
