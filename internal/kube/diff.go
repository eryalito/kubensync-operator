package kube

import (
	"reflect"
	"sort"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
)

func AreManagedResourcesStatusDifferent(mr1, mr2 automationv1alpha1.ManagedResourceStatus) bool {
	sort.SliceStable(mr1.CreatedResources, func(i, j int) bool {
		return mr1.CreatedResources[i].UID < mr1.CreatedResources[j].UID
	})
	sort.SliceStable(mr2.CreatedResources, func(i, j int) bool {
		return mr2.CreatedResources[i].UID < mr2.CreatedResources[j].UID
	})
	return !reflect.DeepEqual(mr1, mr2)
}
