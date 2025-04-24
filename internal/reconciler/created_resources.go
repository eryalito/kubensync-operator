package reconciler

import (
	"context"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	createdResourcesLogger      = ctrl.Log.WithName("created_resources")
	createdResourcesLoggerDebug = ctrl.Log.WithName("created_resources").V((1))
)

func (r *Reconciler) ReconcileMRCreatedResources(ctx context.Context, managedResource *automationv1alpha1.ManagedResource) error {
	for _, createdResource := range managedResource.Status.CreatedResources {
		// Check if the resource still exists
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(createdResource.ApiVersion)
		obj.SetKind(createdResource.Kind)
		obj.SetName(createdResource.Name)
		obj.SetNamespace(createdResource.Namespace)
		ri, err := kube.GetResourceInterfaceForUnstructured(obj, r.RestConfig)
		if err != nil {
			return err
		}

		_, err = ri.Get(ctx, createdResource.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// Resource not found, remove it from status
				createdResourcesLoggerDebug.Info("Resource not found, removing it from status", "resource", createdResource)
				managedResource.Status.CreatedResources = removeResource(managedResource.Status.CreatedResources, createdResource)
			} else {
				return err
			}
		} else {
			// Logic starts here

			triggerNamespaceObj, err := r.Clientset.CoreV1().Namespaces().Get(ctx, createdResource.TriggerNamespace, metav1.GetOptions{})
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			// if trigger the trigger namespace does not exist, remove the resource
			if errors.IsNotFound(err) {
				// Trigger namespace not found, remove the resource
				createdResourcesLogger.Info("Trigger namespace not found, removing resource", "triggerNamespace", createdResource.TriggerNamespace, "resource", createdResource)
				err = ri.Delete(ctx, createdResource.Name, metav1.DeleteOptions{})
				if err != nil {
					return err
				}
				managedResource.Status.CreatedResources = removeResource(managedResource.Status.CreatedResources, createdResource)
				createdResourcesLogger.Info("Resource removed", "resource", createdResource)
				continue
			}

			// if the resource trigger namespace no longer matches the managed resource trigger namespace, delete the resource

			matches, err := namespaceMatchesManagedResource(triggerNamespaceObj, managedResource)
			if err != nil {
				reconcilerLogger.Error(err, "Error matching namespace with managed resource")
				return err
			}

			if !matches {
				// Trigger namespace does not match, remove the resource
				createdResourcesLogger.Info("Trigger namespace does not match, removing resource", "triggerNamespace", createdResource.TriggerNamespace, "resource", createdResource)
				err = ri.Delete(ctx, createdResource.Name, metav1.DeleteOptions{})
				if err != nil {
					return err
				}
				managedResource.Status.CreatedResources = removeResource(managedResource.Status.CreatedResources, createdResource)
				createdResourcesLogger.Info("Resource removed", "resource", createdResource)
			}

		}
		err = kube.UpdateStatus(managedResource, ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
