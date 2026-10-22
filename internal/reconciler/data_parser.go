package reconciler

import (
	"context"
	"fmt"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	dataPerserLogger      = ctrl.Log.WithName("data_parser")
	dataPerserLoggerDebug = ctrl.Log.WithName("data_parser").V((1))
)

func (r *Reconciler) getTemplateData(data []automationv1alpha1.ManagedResourceSpecTemplateData) (map[string]interface{}, error) {
	var err error
	parsedData := make(map[string]interface{})
	for _, dataelement := range data {
		dataPerserLoggerDebug.Info("Parsing data element", "name", dataelement.Name, "type", dataelement.Type)
		var refData map[string]interface{}
		switch dataelement.Type {
		case automationv1alpha1.Secret:
			refData, err = parseSecretData(dataelement.Ref, r.Clientset)
		case automationv1alpha1.ConfigMap:
			refData, err = parseCMData(dataelement.Ref, r.Clientset)
		case automationv1alpha1.KubernetesResource:
			refData, err = parseKubernetesResourceData(dataelement.Ref, r.RESTMapper, r.DynamicClient)
		default:
			err = fmt.Errorf("unsupported data type: %s", dataelement.Type)
		}
		if err != nil {
			dataPerserLogger.Error(err, "Error parsing ref")
			return nil, err
		}
		parsedData[dataelement.Name] = refData
	}
	return parsedData, nil
}

func parseKubernetesResourceData(managedResourceSpecTemplateDataRef automationv1alpha1.ManagedResourceSpecTemplateDataRef, mapper meta.RESTMapper, dyn dynamic.Interface) (map[string]interface{}, error) {
	unstructuredObj := &unstructured.Unstructured{}
	unstructuredObj.SetName(managedResourceSpecTemplateDataRef.Name)
	unstructuredObj.SetNamespace(managedResourceSpecTemplateDataRef.Namespace)
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   managedResourceSpecTemplateDataRef.Group,
		Version: managedResourceSpecTemplateDataRef.ApiVersion,
		Kind:    managedResourceSpecTemplateDataRef.Kind,
	})
	ri, err := kube.GetResourceInterfaceForUnstructured(unstructuredObj, mapper, dyn)
	if err != nil {
		dataPerserLogger.Error(err, "Error getting resource interface")
		return nil, err
	}
	resource, err := ri.Get(context.TODO(), managedResourceSpecTemplateDataRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			dataPerserLogger.Error(err, "Resource not found")
			return nil, nil
		}
		dataPerserLogger.Error(err, "Error retrieving resource")
		return nil, err
	}

	// Convert the resource object to YAML
	yamlData, err := yaml.Marshal(resource.Object)
	if err != nil {
		dataPerserLogger.Error(err, "Error converting resource to YAML")
		return nil, err
	}

	// Unmarshal the YAML back into a map[string]interface{}
	data := make(map[string]interface{})
	err = yaml.Unmarshal(yamlData, &data)
	if err != nil {
		dataPerserLogger.Error(err, "Error unmarshaling YAML to map")
		return nil, err
	}

	return data, nil
}

func parseSecretData(ref automationv1alpha1.ManagedResourceSpecTemplateDataRef, clientset *kubernetes.Clientset) (map[string]interface{}, error) {
	secret, err := clientset.CoreV1().Secrets(ref.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		dataPerserLogger.Error(err, "Error retrieving Secret")
		return nil, err
	}
	data := make(map[string]interface{})
	for key, value := range secret.Data {
		data[key] = string(value)
	}
	return data, nil
}

func parseCMData(ref automationv1alpha1.ManagedResourceSpecTemplateDataRef, clientset *kubernetes.Clientset) (map[string]interface{}, error) {
	configmap, err := clientset.CoreV1().ConfigMaps(ref.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		dataPerserLogger.Error(err, "Error retrieving configmap")
		return nil, err
	}
	data := make(map[string]interface{})
	for key, value := range configmap.Data {
		data[key] = value
	}
	return data, nil
}

// deepCopyData copies the maps and slices of parsed template data; the values they hold are immutable.
func deepCopyData(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = deepCopyValue(value)
	}
	return out
}

func deepCopyValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return deepCopyData(v)
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, element := range v {
			out[i] = deepCopyValue(element)
		}
		return out
	default:
		return v
	}
}

// isTemplateDataRef reports whether obj is one of the resources read as template data.
func isTemplateDataRef(data []automationv1alpha1.ManagedResourceSpecTemplateData, obj *unstructured.Unstructured) bool {
	gvk := obj.GroupVersionKind()
	for _, dataelement := range data {
		group, kind := dataelement.Ref.Group, dataelement.Ref.Kind
		switch dataelement.Type {
		case automationv1alpha1.Secret:
			group, kind = "", "Secret"
		case automationv1alpha1.ConfigMap:
			group, kind = "", "ConfigMap"
		}
		// Cluster-scoped objects are read regardless of the namespace set on the ref.
		sameNamespace := obj.GetNamespace() == "" || obj.GetNamespace() == dataelement.Ref.Namespace
		if gvk.Group == group && gvk.Kind == kind && obj.GetName() == dataelement.Ref.Name && sameNamespace {
			return true
		}
	}
	return false
}
