package reconciler

import (
	"context"
	"regexp"

	"github.com/eryalito/kubensync-operator/api/v1alpha1"
	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
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

func isNamespacePresent(name string, clientset *kubernetes.Clientset) (bool, error) {
	if name == "" {
		return false, nil
	}
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// removeResource removes a CreatedResource from the slice if it exists.
func removeResource(resources []v1alpha1.CreatedResource, target v1alpha1.CreatedResource) []v1alpha1.CreatedResource {
	for i, resource := range resources {
		if resource == target {
			// Remove the element by slicing around it
			return append(resources[:i], resources[i+1:]...)
		}
	}
	return resources // Return unchanged if not found
}
