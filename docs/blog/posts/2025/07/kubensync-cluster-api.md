---
date: 2025-07-15

authors:
  - eryalito
categories:
  - Use-cases
  - Cluster API
---


# KubeNSync when using Cluster API

When managing multiple Kubernetes clusters, using Cluster API (CAPI) for deploying, and ArgoCD, FluxCD or similar tools for deploying applications, is a common scenario. However, managing basic cluster configuration such as CCMs, Pull Secrets, etc. can be hard to manage.

In these cases, using tools like [ClusterResourceSet (CRS)](https://cluster-api.sigs.k8s.io/tasks/cluster-resource-set) or the [Helm chart provider](https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm) can help, but they often require additional steps like building the Helm values, injecting secrets, and potentially depending on external tools like some kind of vault.

<!-- more -->

## Use case: Injecting a pull secret into guest clusters

I faced this problem when deploying air-gapped clusters with a private registry. A pull secret is needed to pull images from the private registry, so there are some options available using _traditional_ methods:

1. **Using ClusterResourceSet**: You can create a `ClusterResourceSet` that creates the resources defined on a ConfigMap into the cluster. This is a viable option at first, but it requires to manually create the ConfigMap with the secret and rotating it for each cluster. This implies that the tool deploying the `ClusterResourceSet` must have access to the secret, which was not my case.
2. **Using Helm**: The scenario using the Helm chart provider is similar to the previous one, but it requires to create a Helm chart with the secret and deploy it to each cluster. This is also a viable option, but, same as before, it requires to know the secret to provide it on the values.
3. **Configuring containerd**: Another option could be to configure containerd to use the pull secret by default. This is a more complex solution, as it requires to modify the containerd configuration on each cluster. Anyway, this is not valid not only because I don't have the secret at deploy time, but also because adding the pull secret _globally_ is not a good practice, as any pod in any namespace could download images. Also this would be limited to only one credential per registry, which is not ideal in many cases.

In summary, I needed a **_generic_ solution** that would allow me to **inject arbitrary secrets** into the guest clusters **without knowing them at deploy time**, they are already present on the host cluster and their contents might change so **it needs to be synchronized**. This is where KubeNSync comes into play.

## Prerequisites

Before we begin, ensure you have the following prerequisites:

- A Kubernetes cluster set up with Cluster API.
- KubeNSync operator installed on your host cluster.
- Guest clusters created and managed by Cluster API.

## Solution

The overall idea is to use KubeNSync to synchronize a pull secret stored in the host cluster to the guest clusters, using as few pieces as possible. For achieving this, I decided to go with the ClusterResourceSet approach as it's CAPI-native and no addons are required. The steps are as follows:

1. The `source-pull-secret` is a Secret in the `kube-system` namespace of the host cluster. It's referenced by a `ManagedResource`.
2. The `ManagedResource` creates a `Secret` and `ClusterResourceSet` on each guest cluster namespace.
3. The CAPI controller will process the `ClusterResourceSet` and get the resources defined on the `Secret`.
4. The CAPI controller will create a Secret in the `kube-system` namespace of the guest cluster.

``` mermaid
graph TD
    subgraph Host Cluster
        subgraph kube-system namespace
            A["source-pull-secret<br>(Secret)"]
        end
        subgraph ClusterScoped resources
            B["capi-pull-secret<br>(ManagedResource)"]
        end
        A -- "(1) Referenced by" --> B
        subgraph capi-cluster-1 namespace
            B -- "(2) Creates" --> C("crs-pull-secret<br>(Secret)")
            B -- "(2) Creates" --> D("crs-pull-secret<br>(ClusterResourceSet)")
        end
        D -- "(3) Managed by" --> E["CAPI<br>Controller"]
    end
    subgraph Cluster-1 Cluster
        E -- "(4) Creates Secret" --> F["pull-secret<br>(kube-system)"]
    end
```

!!! Note
    The flow assumes that each guest cluster resources live in their own namespace. This is not strictly necessary, but it is a common practice when using Cluster API.

## Implementation

First of all, we need to create the `source-pull-secret` in the host cluster. This is a Secret that contains the pull secret that we want to synchronize to the guest clusters.

```yaml title="source-pull-secret.yaml"
apiVersion: v1
kind: Secret
metadata:
  name: source-pull-secret
  namespace: kube-system
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-pull-secret>
```

```bash
kubectl create -f source-pull-secret.yaml
```

Now, we need to create the `ManagedResource` that will create the `Secret` and `ClusterResourceSet` on each guest cluster namespace. This is done by creating a `ManagedResource` that references the `source-pull-secret` and uses a label to identify the CAPI namespaces.

```yaml title="capi-pull-secret.yaml"
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: capi-pull-secret
spec:
  namespaceSelector:
    labelSelector:
      matchLabels:
        capi-pull-secret: "true"
  template:
    data:
      - name: pull_secret
        type: Secret
        ref:
          name: source-pull-secret
          namespace: kube-system
    literal: |
      ---
      apiVersion: v1
      kind: Secret
      metadata:
        name: crs-pull-secret
        namespace: {{ .Namespace.Name }}
      type: addons.cluster.x-k8s.io/resource-set
      stringData:
        manifests.yaml: |
          ---
          apiVersion: v1
          data:
            .dockerconfigjson: '{{ index .Data.pull_secret ".dockerconfigjson" | base64Encode }}'
          kind: Secret
          metadata:
            name: pull-secret
            namespace: kube-system
          type: kubernetes.io/dockerconfigjson
      ---
      apiVersion: addons.cluster.x-k8s.io/v1beta1
      kind: ClusterResourceSet
      metadata:
        name: crs-pull-secret
        namespace: {{ .Namespace.Name }}
      spec:
        clusterSelector:
          matchLabels:
            cluster.x-k8s.io/cluster-name: {{ .Namespace.Name }}
        resources:
        - kind: Secret
          name: crs-pull-secret
        strategy: Reconcile
```

```bash
kubectl create -f capi-pull-secret.yaml
```

And this is almost everything! The `ManagedResource` will create the `Secret` and `ClusterResourceSet` on each guest cluster namespace that matches the label selector. The CAPI controller will then process the `ClusterResourceSet` and create the `pull-secret` in the `kube-system` namespace of each guest cluster.

## Benefits

With this approach all of the points mentioned in the use case are covered:

- **Generic solution**: The `ManagedResource` can be used to synchronize any secret, not just pull secrets. Any resource for that matter.
- **Secret value agnostic**: Both the `source-pull-secret` Secret and `capi-pull-secret` ManagedResource can be created beforehand, so we don't need to know its value at deploy time.
- **Synchronization**: The `ManagedResource` will automatically synchronize the `source-pull-secret` to the `crs-pull-secret`, and CAPI will update it on the guest clusters.

## Caveats

There are a few caveats to keep in mind when using this approach:

- This solution assumes that the guest clusters resources are created in their own namespaces and the namespace name matches the cluster name.

    !!! tip
        If this is not the case, you might need to adjust both the `namespaceSelector` in the ManagedResource spec and the `clusterSelector` in the ClusterResourceSet spec to match your actual environment.

- The `ManagedResource` will create a `ClusterResourceSet` only in the namespaces that match the label selector. So you have to ensure that the namespaces where you want to deploy the pull secret have the label `capi-pull-secret: "true"`.

This work is licensed under a [Creative Commons Attribution 4.0 International License](/blog/LICENSE).
