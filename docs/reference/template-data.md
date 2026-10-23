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

### Watching data

The data is read every time the ManagedResource is reconciled, so by default a change to a data resource only reaches the rendered resources when the ManagedResource is reconciled for another reason. Set `watch: true` on a data entry to reconcile the ManagedResource as soon as that resource is created, changed or deleted.

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
          watch: true
        literal: |
            # Continue...
```

- It works with every type. Only the referenced resource is watched, and only its metadata is kept in memory, so the contents of a Secret are never cached.
- ManagedResources that reference the same resource, or several entries of one ManagedResource that do, share a single watch. The resource is watched if any entry that references it sets `watch: true`.
- Changes made within a couple of seconds of each other are merged into one reconcile.
- The operator needs the `list` and `watch` permissions on watched resources, besides `get`.
- With `avoidResourceUpdate: true`, a reconcile caused by a watched resource only creates missing resources; existing ones are not updated.

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
                key1: {{ index .Data.my_secret "key1" | base64Encode }}
                key2: {{ index .Data.my_secret "key2" | base64Encode }}
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
    The ServiceAccount name is loaded from `.Data.my_resource.metadata.name` as the raw kubernetes object is loaded and not processed.
