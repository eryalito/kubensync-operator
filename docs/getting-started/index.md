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

1. Import the catalog source:
    ``` { .bash }
    kubectl apply -f https://raw.githubusercontent.com/eryalito/operator-catalog/main/samples/catalogsource.yml
    ```
2. Install the `kubensync` operator using [olm](https://olm.operatorframework.io/docs/tasks/install-operator-with-olm/).

### Using kubectl / kustomize

1. Clone this repo:
    ```{ .bash } 
    git clone https://github.com/eryalito/kubensync-operator.git
    ```
2. Change the working directory:
    ``` { .bash }
    cd kubensync-operator
    ```
3. Deploy the operator and its resources:
    ``` { .bash }
    kubectl apply -k deploy/
    ```

## Uninstallation

### Using Operator Lifecycle Manager (OLM)
1. Open the olm in your cluster.
2. Find the kubensync operator and click "Uninstall."

### Using kubectl / kustomize
1. Change the working directory:
    ``` { .bash }
    cd kubensync-operator
    ```
2. Delete the kubensync resources:
    ``` { .bash }
    kubectl delete -k deploy/
    ```