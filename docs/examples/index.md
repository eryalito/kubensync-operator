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
        regex: "^dev-.*"
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
        regex: "^(namespace1|namespace2)$"
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
This MR will create a Role `my-role` in each namespace that contains `namespace1` o `namespace2` in its name that contains the credentials to connect to your internal registry.

## Create default quotas on all non core namespaces
``` { .yaml }
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: default-quotas
spec:
    avoidResourceUpdate: true
    namespaceSelector:
        regex: "^[^k].*|k[^u].*|ku[^b].*" # (1)!
    template:
        literal: |
            ---
            apiVersion: v1
            kind: ResourceQuota
            metadata:
                name: cpu-quota
                namespace: {{ .Namespace.Name }}
            spec:
                hard:
                    cpu: "4"
```

1.  !!! warning
    As Go regex stdlib does not support negative lookaheads the negative expressions is a bit funny. It would be `^(?!kube-).*`, meaning everything that does not start by `kube-`.
    
This MR will create a ResourceQuota `cpu-quota` in each namespace that not start with `kube-` with cpu hard value of `4`, but it will not be resynced unless it's deleted, so the quota can be edited by other means and it won't be restored to the default `4`. 

