# Creating a ServiceAccount in All Test Namespaces

## Use case

A specific Service Account is required in all test namespaces. This is need to run test jobs in a CI/CD pipeline and the default SA is not allowed.

## Implementation

This ManagedResource (MR) will create a Service Account `managed-resource-sa` in each namespace that contains `test` in its name.

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
