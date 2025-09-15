# Getting Started

## Prerequisites

Before deploying the kubensync operator, ensure you have the following prerequisites:

- Kubernetes cluster up and running.
- `kubectl` CLI tool configured to access your cluster.
- `helm` CLI tool installed (if you choose to deploy using Helm).
- `cluster-admin` privileges.

## Installation

### Using kubectl

Install the operator:

    kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/install.yaml

Grant Permissions:

    kubectl apply -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/rbac.yaml

!!! warning Cluster-admin permissions
    This permissions will grant the operator cluster-admin permissions. It's a good way of testing the operator, but specific permissions should be defined according to the resources it will manage in each specific case.

### Using Helm

Install the operator using the Helm chart:

    helm install kubensync oci://ghcr.io/eryalito/kubensync-charts/kubensync --version 0.9.4 -n kubensync-system --create-namespace --wait

!!! info "Helm Chart"
    To get more information about the Helm chart, check the [Helm Chart documentation](https://github.com/eryalito/kubensync-operator/tree/master/dist/chart)

## Uninstallation

### Using kubectl

Delete the operator:

    kubectl delete -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/install.yaml

Delete Permissions:

    kubectl delete -f https://raw.githubusercontent.com/eryalito/kubensync-operator/master/dist/rbac.yaml

### Using Helm

Uninstall the operator using Helm:

    helm uninstall kubensync -n kubensync-system
