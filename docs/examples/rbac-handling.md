# Setting up RBAC permissions

## Use case

Automatic RBAC permissions applied to namespaces to give users access to specific resources. This is useful for setting up permissions in a multi-tenant environment where different teams or users need access to different resources.

## Implementation

This ManagedResource (MR) will create a Role `my-role` in each namespace that contains `dev-` in its name. The role will allow the user to get, list, and watch pods.

``` { .yaml }
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: rbac-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
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

This can be extended to create a RoleBinding to bind the role to a user or group. For example, to bind the role to a user named `my-user`, you can use the following MR:

``` { .yaml }
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: rbac-binding-sample
spec:
    namespaceSelector:
        regex: "^dev-.*"
    template:
        literal: |
            ---
            apiVersion: rbac.authorization.k8s.io/v1
            kind: RoleBinding
            metadata:
                name: my-role-binding
                namespace: {{ .Namespace.Name }}
            subjects:
                - kind: User
                  name: my-user
                  apiGroup: rbac.authorization.k8s.io
            roleRef:
                kind: Role
                name: my-role
                apiGroup: rbac.authorization.k8s.io
```
