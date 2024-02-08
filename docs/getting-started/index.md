# Getting Started

## Prerequisites

Before deploying the kubensync operator, ensure you have the following prerequisites:

- Kubernetes cluster up and running.
- `kubectl` CLI tool configured to access your cluster.
- `cluster-admin` privileges.
- [Operator Lifecycle Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) installed if you want to use the OLM installation method.

## Installation

!!! warning "Default SA permissions"
    After installing the operator, the operator service account does not have permissions to create resources by default. Therefore, you need to define and grant the necessary permissions manually. This allows you to specify the minimum permission level required for the operator to create objects.

    The reason for this is that the template is rendered at runtime, so it is not possible to determine the required permissions for each specific scenario before installing the operator.

### Using Operator Lifecycle Manager (OLM)

1. Install the `kubensync` operator from the [OperatorHub](https://operatorhub.io/operator/kubensync).

### Using kubectl / kustomize

1. Install the operator:
    ```{ .bash } 
    kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/render/manifests.yaml
    ```
2. Grant Permissions: 
    ``` { .bash }
    kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/render/rbac.yaml
    ```

    !!! warning Cluster-admin permissions
        This permissions will grant the operator cluster-admin permissions. It's a good way of testing the operator, but specific permissions should be defined acording to the resources it will manage in each specific case.

## Uninstallation

### Using Operator Lifecycle Manager (OLM)
1. Open the olm in your cluster.
2. Find the kubensync operator and click "Uninstall."

### Using kubectl / kustomize

1. Delete the operator:
    ```{ .bash } 
    kubectl delete -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/render/manifests.yaml
    ```
2. Delete Permissions: 
    ``` { .bash }
    kubectl delete -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/render/rbac.yaml
    ```