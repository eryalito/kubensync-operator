package reconciler

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync"
	"text/template"

	automationv1alpha1 "github.com/kubensync/operator/api/v1alpha1"
	"github.com/kubensync/operator/pkg/kube"
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
	mutex            = sync.Mutex{}
	reconcilerLogger = ctrl.Log.WithName("reconciler")
)

func (r *Reconciler) ReconcileNamespaceChange(ctx context.Context, mrDef *automationv1alpha1.ManagedResource, namespace *corev1.Namespace) error {
	mutex.Lock()
	defer mutex.Unlock()

	r.ownerRefs = mrOwnerRefs(mrDef)

	regex, err := regexp.Compile(mrDef.Spec.NamespaceSelector.Regex)
	if err != nil {
		return err
	}
	if !regex.MatchString(namespace.Name) {
		return nil
	}
	reconcilerLogger.Info("Reconciling", "Namespace", namespace.Name, "ManagedResource", mrDef.Name)
	manifests, err := renderTemplateForNamespace(mrDef.Spec.Template, namespace, r.RestConfig)
	if err != nil {
		return err
	}
	manifestList := strings.Split(manifests, "---")
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
			return err
		}

		metadata := obj.Object["metadata"].(map[string]interface{})
		metadata["ownerReferences"] = mrOwnerRefs(mrDef)
		_, err = ri.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			if errors.IsAlreadyExists(err) {
				_, err = ri.Update(ctx, obj, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
			}
			return err
		}
	}
	reconcilerLogger.Info("End reconciling", "Namespace", namespace.Name, "ManagedResource", mrDef.Name)
	return nil
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

func renderTemplateForNamespace(tpl automationv1alpha1.ManagedResourceSpecTemplate, namespace *corev1.Namespace, config *rest.Config) (string, error) {
	tmpl, err := template.New("").Parse(tpl.Literal)
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
	secretData := make(map[string]interface{})
	for key, value := range secret.Data {
		secretData[key] = string(value)
	}
	return secretData, nil
}

func parseCMData(ref automationv1alpha1.ManagedResourceSpecTemplateDataRef, clientset *kubernetes.Clientset) (map[string]interface{}, error) {
	secret, err := clientset.CoreV1().ConfigMaps(ref.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		reconcilerLogger.Error(err, "Error retrieving Secret")
		return nil, err
	}
	secretData := make(map[string]interface{})
	for key, value := range secret.Data {
		secretData[key] = string(value)
	}
	return secretData, nil
}
