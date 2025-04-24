package reconciler

import (
	"regexp"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func namespaceMatchesManagedResource(namespace *corev1.Namespace, managedResource *automationv1alpha1.ManagedResource) (bool, error) {
	regex, err := regexp.Compile(managedResource.Spec.NamespaceSelector.Regex)
	if err != nil {
		return false, err
	}
	if !regex.MatchString(namespace.Name) {
		return false, nil
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(&managedResource.Spec.NamespaceSelector.LabelSelector)
	if err != nil {
		return false, err
	}
	namespaceLabels := labels.Set(namespace.GetLabels())
	if !labelSelector.Matches(namespaceLabels) {
		return false, nil
	}
	return true, nil
}

// removeResource removes a CreatedResource from the slice if it exists.
func removeResource(resources []automationv1alpha1.CreatedResource, target automationv1alpha1.CreatedResource) []automationv1alpha1.CreatedResource {
	for i, resource := range resources {
		if resource == target {
			// Remove the element by slicing around it
			return append(resources[:i], resources[i+1:]...)
		}
	}
	return resources // Return unchanged if not found
}
