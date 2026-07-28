package reconciler

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"text/template"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/all"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Reconciler struct {
	Clientset  *kubernetes.Clientset
	ownerRefs  []metav1.OwnerReference
	RestConfig *rest.Config
}

var (
	Mutex                 = sync.Mutex{}
	reconcilerLogger      = ctrl.Log.WithName("reconciler")
	reconcilerLoggerDebug = ctrl.Log.WithName("reconciler").V((1))
)

// maxTemplateDocuments bounds the documents accepted from a single rendered
// template, so a decoder that stops consuming input cannot loop indefinitely.
const maxTemplateDocuments = 1024

// nolint:gocyclo // Ignore gocyclo for this line
func (r *Reconciler) ReconcileNamespaceChange(ctx context.Context, mrDef *automationv1alpha1.ManagedResource, namespace *corev1.Namespace) (result *automationv1alpha1.ManagedResource, err error) {
	newMRDef := mrDef.DeepCopy()
	r.ownerRefs = mrOwnerRefs(mrDef)

	matches, err := namespaceMatchesManagedResource(namespace, mrDef)
	if err != nil {
		reconcilerLogger.Error(err, "Error matching namespace with managed resource")
		return nil, err
	}

	if !matches {
		return newMRDef, nil
	}

	manifests, err := renderTemplateForNamespace(mrDef.Spec.Template, namespace, r.RestConfig)
	if err != nil {
		return nil, err
	}
	objs, err := decodeManifests(manifests)
	if err != nil {
		return nil, err
	}

	remainingPrevCreatedResources := mrDef.Status.CreatedResources
	createdAndUpdatedResourcesList := []automationv1alpha1.CreatedResource{}

	// On any failure while touching the API server, report the resources created so
	// far plus the previously tracked ones not yet confirmed gone, so nothing is
	// dropped from the status or left listed after being deleted. Both loops keep
	// remainingPrevCreatedResources in sync so this holds at every exit.
	defer func() {
		if err != nil {
			newMRDef.Status.CreatedResources = append(createdAndUpdatedResourcesList, remainingPrevCreatedResources...)
			result = newMRDef
		}
	}()

	for _, obj := range objs {
		ri, err := kube.GetResourceInterfaceForUnstructured(obj, r.RestConfig)
		if err != nil {
			return nil, err
		}

		metadata := obj.Object["metadata"].(map[string]interface{})
		metadata["ownerReferences"] = mrOwnerRefs(mrDef)
		getObj, err := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
		}
		uid := ""
		if getObj != nil {
			uid = string(getObj.GetUID())
			metadata["ownerReferences"] = appendOwnerReference(getObj.GetOwnerReferences(), mrOwnerRefs(mrDef)[0])
		}

		if getObj == nil {
			reconcilerLoggerDebug.Info("Creating resource", "Namespace", obj.GetNamespace(), "Name", obj.GetName(), "Kind", obj.GetKind(), "ApiVersion", obj.GetAPIVersion())
			uns, err := ri.Create(ctx, obj, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}
			uid = string(uns.GetUID())
		} else if !mrDef.Spec.AvoidResourceUpdate {
			obj.SetResourceVersion(getObj.GetResourceVersion())
			obj.SetUID(getObj.GetUID())
			if !reflect.DeepEqual(getObj, obj) {
				reconcilerLoggerDebug.Info("Updating resource", "Namespace", obj.GetNamespace(), "Name", obj.GetName(), "Kind", obj.GetKind(), "ApiVersion", obj.GetAPIVersion())
				_, err = ri.Update(ctx, obj, metav1.UpdateOptions{})
				if err != nil {
					return nil, err
				}
			}
		}
		createdObject := automationv1alpha1.CreatedResource{
			ApiVersion:       obj.GetAPIVersion(),
			Kind:             obj.GetKind(),
			Name:             obj.GetName(),
			Namespace:        obj.GetNamespace(),
			UID:              uid,
			TriggerNamespace: namespace.Name,
		}
		createdAndUpdatedResourcesList = append(createdAndUpdatedResourcesList, createdObject)

		// remove created resource from the list of previously created resources, so we can delete the ones that are not needed anymore
		for i, prevResource := range remainingPrevCreatedResources {
			// If both resources are cluster-scoped
			if prevResource.Namespace == "" && createdObject.Namespace == "" && prevResource.Name == createdObject.Name && prevResource.ApiVersion == createdObject.ApiVersion && prevResource.Kind == createdObject.Kind {
				remainingPrevCreatedResources = append(remainingPrevCreatedResources[:i], remainingPrevCreatedResources[i+1:]...)
				break
			}
			// If both resources are namespace-scoped
			if prevResource.Namespace != "" && createdObject.Namespace != "" && prevResource.Name == createdObject.Name && prevResource.Namespace == createdObject.Namespace && prevResource.ApiVersion == createdObject.ApiVersion && prevResource.Kind == createdObject.Kind {
				remainingPrevCreatedResources = append(remainingPrevCreatedResources[:i], remainingPrevCreatedResources[i+1:]...)
				break
			}
		}
	}

	// Delete the remaining resources that were created in the previous reconciliation but are not needed anymore.
	// Each entry is removed from remainingPrevCreatedResources once it is confirmed gone or kept, so the deferred
	// status update keeps reflecting what still exists if a delete fails partway.
	for len(remainingPrevCreatedResources) > 0 {
		resource := remainingPrevCreatedResources[0]
		// The trigger namespace should be the same, if not, just skip it and keep it as created
		if resource.TriggerNamespace != namespace.Name {
			createdAndUpdatedResourcesList = append(createdAndUpdatedResourcesList, resource)
			remainingPrevCreatedResources = remainingPrevCreatedResources[1:]
			continue
		}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(resource.ApiVersion)
		obj.SetKind(resource.Kind)
		obj.SetName(resource.Name)
		obj.SetNamespace(resource.Namespace)
		ri, err := kube.GetResourceInterfaceForUnstructured(obj, r.RestConfig)
		if err != nil {
			return nil, err
		}
		reconcilerLoggerDebug.Info("Deleting resource", "Namespace", obj.GetNamespace(), "Name", obj.GetName(), "Kind", obj.GetKind(), "ApiVersion", obj.GetAPIVersion())
		err = ri.Delete(ctx, resource.Name, metav1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				reconcilerLoggerDebug.Info("Resource already deleted", "Namespace", obj.GetNamespace(), "Name", obj.GetName(), "Kind", obj.GetKind(), "ApiVersion", obj.GetAPIVersion())
				remainingPrevCreatedResources = remainingPrevCreatedResources[1:]
				continue
			}
			return nil, err
		}
		remainingPrevCreatedResources = remainingPrevCreatedResources[1:]
	}

	newMRDef.Status.CreatedResources = createdAndUpdatedResourcesList

	reconcilerLogger.Info("End reconciling", "Namespace", namespace.Name, "ManagedResource", mrDef.Name)
	return newMRDef, nil
}

// decodeManifests decodes every document of a rendered template.
func decodeManifests(manifests string) ([]*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifests), 1024)
	objs := []*unstructured.Unstructured{}
	for document := 1; ; document++ {
		if document > maxTemplateDocuments {
			return nil, fmt.Errorf("rendered template holds more than %d documents", maxTemplateDocuments)
		}
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			reconcilerLogger.Error(err, "Error decoding manifests", "document", document)
			return nil, fmt.Errorf("error decoding rendered manifest document %d", document)
		}
		if len(obj.Object) == 0 {
			continue
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func mrOwnerRefs(rbacDef *automationv1alpha1.ManagedResource) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(rbacDef, schema.GroupVersionKind{
			Group:   automationv1alpha1.GroupVersion.Group,
			Version: automationv1alpha1.GroupVersion.Version,
			Kind:    "ManagedResource",
		}),
	}
}

// appendOwnerReference appends a new OwnerReference to the list of OwnerReferences if it is not already present
func appendOwnerReference(list []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	duplicated := false
	for _, element := range list {
		if element.APIVersion == ref.APIVersion && element.Kind == ref.Kind && element.Name == ref.Name {
			duplicated = true
			break
		}
	}
	if !duplicated {
		list = append(list, ref)
	}
	return list
}

func renderTemplateForNamespace(tpl automationv1alpha1.ManagedResourceSpecTemplate, namespace *corev1.Namespace, config *rest.Config) (string, error) {

	handler := sprout.New()
	// Add all the registries to the handler
	err := handler.AddGroups(all.RegistryGroup())
	if err != nil {
		reconcilerLogger.Error(err, "Error adding registries")
		return "", err
	}

	tmpl, err := template.New("").Funcs(handler.Build()).Parse(tpl.Literal)
	if err != nil {
		return "", err
	}

	refdata, err := getTemplateData(tpl.Data, config)
	if err != nil {
		return "", err
	}

	data := struct {
		Namespace corev1.Namespace `json:"namespace"`
		Data      map[string]interface{}
	}{
		Namespace: *namespace.DeepCopy(),
		Data:      refdata,
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
