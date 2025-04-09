# KubeNSync operator
Welcome to KubeNSync, your go-to Kubernetes operator for simplifying resource management across namespaces and the entire cluster. Say goodbye to repetitive resource creation and hello to efficient deployment with KubeNSync!

## What is KubeNSync?
KubeNSync is a powerful Kubernetes operator designed to make your life easier. It allows you to craft resource templates tailored to your specific needs and effortlessly synchronize them across selected namespaces or even cluster-wide. With KubeNSync, you can streamline resource management and enhance deployment efficiency using its intuitive approach.

[**Check out the documentation**](https://eryalito.github.io/kubensync-operator/) to learn more about how to use KubeNSync.

## Key features
- **Namespace Selector**: Easily choose specific namespaces using a regex or label selectors.
- **Resource Templates**: Create custom resources based on your own templates.
- **Effortless Synchronization**: Your resources are syncronized by default to maintain the desired status.

## Getting Started

### Install the operator using kubectl

``` bash
kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/install.yaml
```

Grant default permissions [more info](https://docs.kubensync.com/getting-started/#installation):

``` bash
kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/rbac.yaml
```

### Create your first ManagedResource

When creating the following ManagedResource a new service account named `managed-resource-sa` will be created inside each namespace that contains `test` on its name:
``` bash
cat <<EOF | kubectl apply -f -
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: serviceaccount-sample
spec:
    namespaceSelector:
        regex: "test"
    template:
        literal: |
            ---
            apiVersion: v1
            kind: ServiceAccount
            metadata:
                name: managed-resource-sa
                namespace: {{ .Namespace.Name }}
EOF
```

For more examples check out [the docs](https://docs.kubensync.com/examples/)
