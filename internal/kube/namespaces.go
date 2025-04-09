package kube

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func GetNamespaces(ctx context.Context, config *rest.Config) (*corev1.NamespaceList, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return namespaces, nil
}

// GetNamespacesByLabel retrieves all namespaces that match the given label selector.
func GetNamespacesByLabel(ctx context.Context, config *rest.Config, selector metav1.LabelSelector) (*corev1.NamespaceList, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	labelSelectorString := metav1.FormatLabelSelector(&selector)
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelectorString,
	}

	return clientset.CoreV1().Namespaces().List(ctx, listOptions)
}
