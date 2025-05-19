package reconciler

import (
	"context"
	"fmt"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/kube"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	dataPerserLogger      = ctrl.Log.WithName("data_parser")
	dataPerserLoggerDebug = ctrl.Log.WithName("data_parser").V((1))
)

func getTemplateData(data []automationv1alpha1.ManagedResourceSpecTemplateData, config *rest.Config) (map[string]interface{}, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		dataPerserLogger.Error(err, "Error creating Kubernetes client")
		return nil, err
	}
	parsedData := make(map[string]interface{})
	for _, dataelement := range data {
		// Retrieve the Secret.
		var refData map[string]interface{}
		switch dataelement.Type {
		case automationv1alpha1.Secret:
			refData, err = parseSecretData(dataelement.Ref, clientset)
		case automationv1alpha1.ConfigMap:
			refData, err = parseCMData(dataelement.Ref, clientset)
		case automationv1alpha1.KubernetesResource:
			refData, err = parseKubernetesResourceData(dataelement.Ref, config)
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

func parseKubernetesResourceData(managedResourceSpecTemplateDataRef automationv1alpha1.ManagedResourceSpecTemplateDataRef, config *rest.Config) (map[string]interface{}, error) {
	unstructuredObj := &unstructured.Unstructured{}
	unstructuredObj.SetName(managedResourceSpecTemplateDataRef.Name)
	unstructuredObj.SetNamespace(managedResourceSpecTemplateDataRef.Namespace)
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   managedResourceSpecTemplateDataRef.Group,
		Version: managedResourceSpecTemplateDataRef.ApiVersion,
		Kind:    managedResourceSpecTemplateDataRef.Kind,
	})
	ri, err := kube.GetResourceInterfaceForUnstructured(unstructuredObj, config)
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
