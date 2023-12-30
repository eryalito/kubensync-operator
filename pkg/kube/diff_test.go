package kube_test

import (
	"testing"

	automationv1alpha1 "github.com/kubensync/operator/api/v1alpha1"
	"github.com/kubensync/operator/pkg/kube"
)

func TestAreManagedResourcesStatusDifferent(t *testing.T) {
	tests := []struct {
		name string
		mr1  automationv1alpha1.ManagedResourceStatus
		mr2  automationv1alpha1.ManagedResourceStatus
		want bool
	}{
		{
			name: "same status",
			mr1: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-2",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
				},
			},
			mr2: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-2",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
				},
			},
			want: false,
		},
		{
			name: "same status, different order",
			mr1: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-2",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
				},
			},
			mr2: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-2",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
				},
			},
			want: false,
		},
		{
			name: "different status",
			mr1: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-2",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
				},
			},
			mr2: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-3",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
				},
			},
			want: true,
		},
		{
			name: "different status, missing one",
			mr1: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-2",
						Namespace:        "default",
						UID:              "2",
						TriggerNamespace: "default",
					},
				},
			},
			mr2: automationv1alpha1.ManagedResourceStatus{
				CreatedResources: []automationv1alpha1.CreatedResource{
					{
						ApiVersion:       "v1",
						Kind:             "Pod",
						Name:             "pod-1",
						Namespace:        "default",
						UID:              "1",
						TriggerNamespace: "default",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kube.AreManagedResourcesStatusDifferent(tt.mr1, tt.mr2); got != tt.want {
				t.Errorf("AreManagedResourcesStatusDifferent() = %v, want %v", got, tt.want)
			}
		})
	}
}
