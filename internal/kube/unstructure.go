package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func GetResourceInterfaceForUnstructured(unstructured *unstructured.Unstructured, mapper meta.RESTMapper, dyn dynamic.Interface) (dynamic.ResourceInterface, error) {
	gvk := unstructured.GetObjectKind().GroupVersionKind()

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	var ri dynamic.ResourceInterface

	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		ri = dyn.Resource(mapping.Resource)
	} else {
		ri = dyn.Resource(mapping.Resource).Namespace(unstructured.GetNamespace())
	}

	return ri, nil
}
