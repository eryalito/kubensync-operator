# Template

The templating engine used by kubensync is based on the Go template engine. It allows you to create dynamic configurations by using placeholders and functions directly integrated in the engine.

The Go template must be provided in the `spec.template.literal` field and the output after rendering must be a list of valid YAML files that can be applied to the Kubernetes cluster, separated by `---`. The template engine will process the placeholders and functions, and generate the final YAML output.

Custom data can be passed to the template using the `data` field. The data is a list of resources that will be injected into the template `.Data` field. The data can be any valid Kubernetes resource, such as a ConfigMap or a Secret. The data is passed to the template as a map under the `.Data` field. The key of the map is the name provided. The resource can be referenced in the template using the `{{ .Data.<name> }}` syntax.

The template engine also provides a set of functions that can be used to manipulate the data. The functions are available in the template context and can be used to perform operations such as encoding, decoding, and formatting the data.

!!! info
    The template engine is based on the Go template engine, so you can use any valid Go template syntax. For more information about the Go template syntax, see the [Go template documentation](https://golang.org/pkg/text/template/).

## Example

Loading and processing a secret and using it in the template:

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    avoidResourceUpdate: false
    namespaceSelector:
        regex: "test"
    template:
        data:
        - name: pull_secret
          type: Secret
          ref:
            name: my-pull-secret
            namespace: default
        literal: |
            ---
            apiVersion: v1
            kind: Secret
            metadata:
                name: my-pull-secret
                namespace: {{ .Namespace.Name }}
            type: kubernetes.io/dockerconfigjson
            data:
                .dockerconfigjson: '{{ index .Data.pull_secret ".dockerconfigjson" | base64Encode }}'
            ---
            # Other resources can be added here
```
