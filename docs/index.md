# Welcome to KubeNSync

KubeNSync is a Kubernetes operator designed to simplify resource management across namespaces and the entire cluster. Say goodbye to repetitive resource creation and hello to efficient deployment with KubeNSync!

---

## What is KubeNSync?

KubeNSync automate the creation of Kubernetes resources using custom templates and namespace selectors. It allows to define a template for a resource and automatically apply it to multiple namespaces, ensuring consistency and reducing manual effort.

### Key Features

- **Namespace Selector**: Use regex or label selectors to target specific namespaces.
- **Custom Resource Templates**: Define reusable templates for Kubernetes resources.
- **Data Injection**: Inject data from existing resources into your templates.
- **Cluster-Wide Synchronization**: Automatically synchronize resources to maintain the desired state.

---

## Getting Started

Ready to dive in? Follow these steps to get started with KubeNSync:

1. **Install the Operator**  
   Deploy KubeNSync to your cluster using `kubectl`:

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/install.yaml
    ```

2. **Grant Permissions**  
   Apply the default RBAC configuration:

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/rbac.yaml
    ```

---

## Documentation

Explore the documentation to learn more:

- [Get Started](getting-started/index.md)
- [Usage](usage/index.md)
- [Reference](reference/index.md)
- [Examples](examples/index.md)