package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

func GetResourceInterfaceForUnstructured(unstructured *unstructured.Unstructured, config *rest.Config) (dynamic.ResourceInterface, error) {
	gvk := unstructured.GetObjectKind().GroupVersionKind()

	dc := discovery.NewDiscoveryClientForConfigOrDie(config)
	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return nil, err
	}
	rm := restmapper.NewDiscoveryRESTMapper(gr)
	mapping, err := rm.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(config)
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
