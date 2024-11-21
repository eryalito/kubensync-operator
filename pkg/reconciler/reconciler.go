package reconciler

import (
	"bytes"
	"context"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"
	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/registry/conversion"
	"github.com/go-sprout/sprout/registry/encoding"
	"github.com/go-sprout/sprout/registry/std"
	automationv1alpha1 "github.com/kubensync/operator/api/v1alpha1"
	"github.com/kubensync/operator/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
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

func (r *Reconciler) ReconcileNamespaceChange(ctx context.Context, mrDef *automationv1alpha1.ManagedResource, namespace *corev1.Namespace) (*automationv1alpha1.ManagedResource, error) {
	newMRDef := mrDef.DeepCopy()
	r.ownerRefs = mrOwnerRefs(mrDef)

	regex, err := regexp.Compile(mrDef.Spec.NamespaceSelector.Regex)
	if err != nil {
		return nil, err
	}
	if !regex.MatchString(namespace.Name) {
		return newMRDef, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&newMRDef.Spec.NamespaceSelector.LabelSelector)
	if err != nil {
		return nil, err
	}
	namespaceLabels := labels.Set(namespace.GetLabels())
	if !labelSelector.Matches(namespaceLabels) {
		// The namespace's labels do not match the label selector
		return newMRDef, nil
	}

	manifests, err := renderTemplateForNamespace(mrDef.Spec.Template, namespace, r.RestConfig)
	if err != nil {
		return nil, err
	}
	manifestList := strings.Split(manifests, "---")
	remainingPrevCreatedResources := mrDef.Status.CreatedResources
	createdAndUpdatedResourcesList := []automationv1alpha1.CreatedResource{}
	for _, manifest := range manifestList {
		if len(manifest) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{}

		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 1024)
		if err := decoder.Decode(obj); err != nil {
			reconcilerLogger.Error(err, "Error deconding manifests")
			continue
		}

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

	// Delete the remaining resources that were created in the previous reconciliation but are not needed anymore
	for _, resource := range remainingPrevCreatedResources {
		// The trigger namespace should be the same, if not, just skip it and keep it as created
		if resource.TriggerNamespace != namespace.Name {
			createdAndUpdatedResourcesList = append(createdAndUpdatedResourcesList, resource)
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
		ri.Delete(ctx, resource.Name, metav1.DeleteOptions{})
	}

	newMRDef.Status.CreatedResources = createdAndUpdatedResourcesList

	reconcilerLogger.Info("End reconciling", "Namespace", namespace.Name, "ManagedResource", mrDef.Name)
	return newMRDef, nil
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
	handler.AddRegistries(std.NewRegistry(), conversion.NewRegistry(), encoding.NewRegistry())

	tmpl, err := template.New("").Funcs(sprig.FuncMap()).Funcs(handler.Build()).Parse(tpl.Literal)
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

func getTemplateData(data []automationv1alpha1.ManagedResourceSpecTemplateData, config *rest.Config) (map[string]interface{}, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		reconcilerLogger.Error(err, "Error creating Kubernetes client")
		return nil, err
	}
	parsedData := make(map[string]interface{})
	for _, dataelement := range data {
		// Retrieve the Secret.
		var refData map[string]interface{}
		if dataelement.Type == automationv1alpha1.Secret {
			refData, err = parseSecretData(dataelement.Ref, clientset)
		} else if dataelement.Type == automationv1alpha1.ConfigMap {
			refData, err = parseCMData(dataelement.Ref, clientset)
		}
		if err != nil {
			reconcilerLogger.Error(err, "Error parsing ref")
			return nil, err
		}
		parsedData[dataelement.Name] = refData
	}
	return parsedData, nil
}

func parseSecretData(ref automationv1alpha1.ManagedResourceSpecTemplateDataRef, clientset *kubernetes.Clientset) (map[string]interface{}, error) {
	secret, err := clientset.CoreV1().Secrets(ref.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		reconcilerLogger.Error(err, "Error retrieving Secret")
		return nil, err
	}
	data := make(map[string]interface{})
	for key, value := range secret.Data {
		data[key] = string(value)
	}
	return data, nil
}

func parseCMData(ref automationv1alpha1.ManagedResourceSpecTemplateDataRef, clientset *kubernetes.Clientset) (map[string]interface{}, error) {
	secret, err := clientset.CoreV1().ConfigMaps(ref.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		reconcilerLogger.Error(err, "Error retrieving Secret")
		return nil, err
	}
	data := make(map[string]interface{})
	for key, value := range secret.Data {
		data[key] = string(value)
	}
	return data, nil
}
