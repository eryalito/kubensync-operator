# Creating a ResourceQuota in All Non-Core Namespaces

## Use case

To avoid overconsumption of resources in a Kubernetes cluster, it is important to set up resource quotas in all namespace in non-core namespaces. When having a multi-tenant cluster, it is important to set up resource quotas in all namespaces, specially on non-production ones.

## Implementation

This ManagedResource (MR) will create a ResourceQuota `default-quota` in each namespace that does not start with `kube`, `kub`, or `k` (core namespaces).

``` { .yaml }
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: default-quota
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
    As Go regex stdlib does not support negative lookaheads the negative expressions is a bit funny. It would be `^(?!kube-).*`.
