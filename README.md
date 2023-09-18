# KubeNSync operator
Introducing KubeNSync, a Kubernetes operator designed to simplify the creation of repetitive resources within distinct namespaces (but also cluster wide!). Craft resource templates tailored to your needs, and with the flexibility of a namespace selector, effortlessly synchronize chosen resources. Streamline resource management and enhance deployment efficiency using KubeNSync's intuitive approach.

## Description
This Kubernetes Operator, named "kubensync", allows users to automate the creation of Kubernetes resources based on Go templates in specified namespaces. It provides a flexible way to automate tasks such as creating pull secrets, setting up RBAC rules, and installing operators. Users can define a Custom Resource (CR) that contains the template to be rendered and a namespace selector in the form of a regex. The operator will then create and manage the specified resources in the selected namespaces.

## Table of Contents

- [KubeNSync operator](#kubensync-operator)
  - [Description](#description)
  - [Table of Contents](#table-of-contents)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
    - [Using Operator Lifecycle Manager (OLM)](#using-operator-lifecycle-manager-olm)
    - [Using kustomize](#using-kustomize)
  - [Usage](#usage)
    - [Custom Resource Definition](#custom-resource-definition)
    - [Examples](#examples)
  - [Uninstallation](#uninstallation)
    - [Using Operator Lifecycle Manager (OLM)](#using-operator-lifecycle-manager-olm-1)
    - [Using Kustomize](#using-kustomize-1)
  - [License](#license)

## Prerequisites

Before deploying the kubensync operator, ensure you have the following prerequisites:

- Kubernetes cluster up and running.
- `kubectl` CLI tool configured to access your cluster.
- `cluster-admin` privileges.
- [Operator Lifecycle Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) installed if you want to use the OLM installation method.
- [Kustomize](https://kustomize.io/) if you want to use the kustomize installation method.

## Installation

### Using Operator Lifecycle Manager (OLM)

1. Import the catalog source:
    ```bash
    kubectl apply -f https://raw.githubusercontent.com/eryalito/operator-catalog/main/samples/catalogsource.yml
    ```
2. Install the `kubensync` operator from the OperatorHub

### Using kustomize

1. Clone this repo:
    ```bash
    git clone https://github.com/eryalito/kubensync.git
    ```
2. Change the working directory:
    ```bash
    cd kubensync
    ```
3. Deploy the operator and its resources:
    ```bash
    kubectl apply -k deploy/
    ```

## Usage

Once the kubensync operator is installed, you can start using it by defining custom resources (CRs) that specify the resources you want to create in selected namespaces.

### Custom Resource Definition

The following is an example of a custom resource (CR) definition:

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: managedresource-sample
spec:
  avoidResourceUpdate: false
  data:
    - name: my_secret
      type: Secret
      ref:
        name: my-secret
        namespace: default
  namespaceSelector:
    regex: "test"
  template:
    literal: |
      ---
      apiVersion: v1
      kind: ServiceAccount
      annotations:
        secret_value: {{ .Data.my_secret.password }}
      metadata:
        name: managed-resource-sa
        namespace: {{ .Namespace.Name }}
```
- `avoidResourceUpdate`: Optional field that changes the default behavior of reconciling existing resources with the desired state. If set to true only non-existing resources will be created an never updated. Default values is `false`.
- `data`: Optional field that read `Secret` or `ConfigMap` and imports the contents to be used in the `template` under `.Data.<name>`.
- `namespaceSelector`: Specifies the namespaces where you want to apply the template. You can use a regular expression (regex) to match multiple namespaces.
- `template`: Contains the YAML template that you want to apply to the selected namespaces. You can use Go template syntax to customize the resource based on the namespace.

### Examples

Here are some example use cases for the kubensync operator:

1. **Creating a ServiceAccount in All Test Namespaces**:
    ```yaml
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
    ```
2. **Creating a Pull Secret in All Development Namespaces**:
    ```yaml
    apiVersion: automation.kubensync.com/v1alpha1
    kind: ManagedResource
    metadata:
      name: pullsecret-sample
    spec:
      namespaceSelector:
        regex: "dev-.*"
    template:
      literal: |
        ---
        apiVersion: v1
        kind: Secret
        metadata:
            name: my-pull-secret
            namespace: {{ .Namespace.Name }}
        type: kubernetes.io/dockerconfigjson
        data:
            .dockerconfigjson: <your pull secret in base64>
    ```
3. **Setting Up RBAC Rules in Specific Namespaces**:
    ```yaml
    apiVersion: automation.kubensync.com/v1alpha1
    kind: ManagedResource
    metadata:
      name: rbac-sample
    spec:
      namespaceSelector:
        regex: "(namespace1|namespace2)"
    template:
      literal: |
        ---
        apiVersion: rbac.authorization.k8s.io/v1
        kind: Role
        metadata:
            name: my-role
            namespace: {{ .Namespace.Name }}
        rules:
            - apiGroups: [""]
            resources: ["pods"]
            verbs: ["get", "list", "watch"]
    ```

## Uninstallation

### Using Operator Lifecycle Manager (OLM)
1. Open the OperatorHub in your cluster.
2. Find the kubensync operator and click "Uninstall."

### Using Kustomize
1. Change the working directory:
    ```bash
    cd kubensync
    ```
2. Delete the kubensync resources:
    ```bash
    kubectl delete -k deploy/
    ```

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

