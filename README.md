# KubeNSync operator
Welcome to KubeNSync, your go-to Kubernetes operator for simplifying resource management across namespaces and the entire cluster. Say goodbye to repetitive resource creation and hello to efficient deployment with KubeNSync!

## What is KubeNSync?
KubeNSync is a powerful Kubernetes operator designed to make your life easier. It allows you to craft resource templates tailored to your specific needs and effortlessly synchronize them across selected namespaces or even cluster-wide. With KubeNSync, you can streamline resource management and enhance deployment efficiency using its intuitive approach.

## Key features
- **Namespace Selector**: Easily choose specific namespaces using a regex.
- **Resource Templates**: Create custom resources based on your own templates.
- **Effortless Synchronization**: Your resources are syncronized by default to maintain the desired status.

## Getting Started
Install the operator:

``` bash
kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/main/render/manifests.yml
```

Grant default permissions [more info](https://docs.kubensync.com/getting-started/#installation):

``` bash
kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/main/render/rbac.yml
```

[**Check out the documentation**](https://eryalito.github.io/kubensync-operator/) to learn more about how to use KubeNSync.
