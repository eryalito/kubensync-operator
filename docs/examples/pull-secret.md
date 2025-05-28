# Creating a Pull Secret in All Development Namespaces

## Use case

Automatic creation of a pull secret in all development namespaces. This is useful for setting up a pull secret on a per-namespace basis, especially in a multi-tenant environment, where each environment may require different credentials to access a private container registry.

## Implementation

This ManagedResource (MR) will clone a pull secret from the default namespace to all namespaces that contain `dev-` in their name.

``` { .yaml }
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: pullsecret-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
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
                .dockerconfigjson: '{{ index .Data.pull_secret ".dockerconfigjson" | base64Encode }}'
```
