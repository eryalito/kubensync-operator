/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Defines the reference to the resource that should be imported.
type ManagedResourceSpecTemplateDataRef struct {
	// Name of the resource.
	Name string `json:"name,omitempty"`
	// Namespace of the resource
	Namespace string `json:"namespace,omitempty"`
}

// Defines the kind of resource the ref is pointing to. Could be `Secret` or `ConfigMap`.
// +enum
type ManagedResourceSpecTemplateType string

const (
	// Secret means that the ref points to a secret.
	Secret ManagedResourceSpecTemplateType = "Secret"

	// ConfigMap means that the ref points to a config map.
	ConfigMap ManagedResourceSpecTemplateType = "ConfigMap"
)

// Describes extra data that will be loaded into the go template as inputs. They all will
// be inside `.Data` parent and all Secret/ConfigMap keys will be loaded. The format inside the template would look as follows
// `.Data.${Name}.${Key}`.
type ManagedResourceSpecTemplateData struct {
	// Name of the key where the contents will be created.
	Name string                             `json:"name,omitempty"`
	Type ManagedResourceSpecTemplateType    `json:"type,omitempty"`
	Ref  ManagedResourceSpecTemplateDataRef `json:"ref,omitempty"`
}

// ManagedResourceSpecNamespaceSelector defines the selector used to specify which namespaces are affected
type ManagedResourceSpecNamespaceSelector struct {
	// Regex that the namespace name must match to be selected
	Regex string `json:"regex,omitempty"`
}

// ManagedResourceSpecTemplate defines the resources to be created when a namespace matches the selector
type ManagedResourceSpecTemplate struct {
	// Literal defines a go template to be renderized for each namespace matching the selector
	Literal string `json:"literal,omitempty"`

	// Data defines a set of refences to secrets or configmaps
	Data []ManagedResourceSpecTemplateData `json:"data,omitempty"`
}

// ManagedResourceSpec defines the desired state of ManagedResource
type ManagedResourceSpec struct {
	NamespaceSelector ManagedResourceSpecNamespaceSelector `json:"namespaceSelector,omitempty"`
	Template          ManagedResourceSpecTemplate          `json:"template,omitempty"`
}

// ManagedResourceStatus defines the observed state of ManagedResource
type ManagedResourceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ManagedResource is the Schema for the managedresources API
type ManagedResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedResourceSpec   `json:"spec,omitempty"`
	Status ManagedResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ManagedResourceList contains a list of ManagedResource
type ManagedResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedResource{}, &ManagedResourceList{})
}
