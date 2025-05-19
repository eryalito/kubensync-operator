# Template Data

Kubernetes objects can be accessed in the template using the `.Data` field. The `.spec.template.data` field contains a list of the resources that are loaded and processed by the template engine. The resources must be present in the Kubernetes cluster and must be accessible to the kubensync operator. The resources are loaded and processed in the order they are defined.

## Data Field

The data resources are defined in the `spec.template.data` field. Each resource must have a unique name. The type can be one of the following:

- `Secret`: A Kubernetes Secret resource.
- `ConfigMap`: A Kubernetes ConfigMap resource.
- `KubernetesResource`: A Kubernetes resource of any kind.

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
    template:
        data:
        - name: my_secret
          type: Secret
          ref:
            name: my-secret
            namespace: default
        - name: my_configmap
          type: ConfigMap
          ref:
            name: my-configmap
            namespace: default
        - name: my_resource
          type: KubernetesResource
          ref:
            apiVersion: v1
            group: ""
            kind: ServiceAccount
            name: test
            namespace: default
        literal: |
            # Continue...
```

### Secret

The `Secret` type is used to load a Kubernetes Secret resource. The `ref` field must contain only the name and namespace of the Secret resource. The Secret resource is loaded and processed, so the keys and values of the Secret are available in the template using the `.Data.<name>` syntax. The values are automatically base64 decoded, so no additional processing is needed.

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
    template:
        data:
        - name: my_secret
          type: Secret
          ref:
            name: my-secret
            namespace: default
        literal: |
            ---
            apiVersion: v1
            kind: Secret
            metadata:
                name: my-secret
                namespace: {{ .Namespace.Name }}
            type: Opaque
            data:
                key1: {{ index .Data.my_secret "key1" | b64enc }}
                key2: {{ index .Data.my_secret "key2" | b64enc }}
```

### ConfigMap

The `ConfigMap` type is used to load a Kubernetes ConfigMap resource. The `ref` field must contain only the name and namespace of the ConfigMap resource. The ConfigMap resource is loaded and processed, so the keys and values of the ConfigMap are available in the template using the `.Data.<name>` syntax.

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
    template:
        data:
        - name: my_configmap
          type: ConfigMap
          ref:
            name: my-configmap
            namespace: default
        literal: |
            ---
            apiVersion: v1
            kind: ConfigMap
            metadata:
                name: my-configmap
                namespace: {{ .Namespace.Name }}
            data:
                key1: {{ index .Data.my_configmap "key1" }}
                key2: {{ index .Data.my_configmap "key2" }}
```

### KubernetesResource

The `KubernetesResource` type is used to load a Kubernetes resource of any kind. The `ref` field must contain the full resource definition, including the `apiVersion`, `group`, `kind`, and `name`. The resource is loaded but not processed. This means that the raw object parse into a map is available in the template using the `.Data.<name>` syntax.

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
    template:
        data:
        - name: my_resource
          type: KubernetesResource
          ref:
            apiVersion: v1
            group: ""
            kind: ServiceAccount
            name: test
            namespace: default
        literal: |
            ---
            apiVersion: v1
            kind: ConfigMap
            metadata:
                name: my-cm
                namespace: {{ .Namespace.Name }}
            data:
                serviceAccountName: '{{ .Data.my_resource.metadata.name }}'
```

!!! tip
    That the ServiceAccount name is loaded from `.Data.my_resource.metadata.name` as the raw kubernetes object is loaded and not processed.
