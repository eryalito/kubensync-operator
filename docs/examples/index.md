KubeNSync can simplify cluster management by creating custom resources tailored to specific scenarios. Here are a couple of examples of how to use KubeNSync to manage different resources:

## Creating a ServiceAccount in All Test Namespaces
``` { .yaml }
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
This MR will create a Service Account `managed-resource-sa` in each namespace that contains `test` in its name.

## Creating a Pull Secret in All Development Namespaces
``` { .yaml }
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
This MR will create a Secret `my-pull-secret` in each namespace that contains `dev-` in its name that contains the credentials to connect to your internal registry.

!!! tip
    References to a valid dockerconfigjson secret to avoid duplicies and having plain secrets can be also used (an recommended!):

    ``` { .yaml }
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
                    .dockerconfigjson: '{{ index .Data.pull_secret ".dockerconfigjson" | b64enc }}'
    ```

## Setting Up RBAC Rules in Specific Namespaces
``` { .yaml }
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
