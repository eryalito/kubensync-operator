package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

// NewRESTMapper returns a mapper that runs discovery on first use and caches it only for its
// own lifetime. Build one per reconcile: a long-lived mapper keeps resolving kinds that are no
// longer served, and the resulting 404s would be read as objects that do not exist.
func NewRESTMapper(dc discovery.DiscoveryInterface) meta.RESTMapper {
	return restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
}

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
