---
date: 2025-08-10

authors:
  - eryalito
categories:
  - Use-cases
  - Security
  - CI/CD
---


# Dynamically configuring RBAC for CD workflows

Handling RBAC (Role-Based Access Control) in Kubernetes can be challenging, especially when you need to dynamically configure permissions for CD tools that use Service Accounts (SAs) across multiple namespaces. We will explore how to configure dynamic RBAC in a real world scenario, taking [ArgoCD](https://argo-cd.readthedocs.io/en/stable/) as an example configuring the necessary permissions for its Service Account in each namespace. This approach can be adapted to other CD tools like [FluxCD](https://fluxcd.io/) or [Jenkins](https://www.jenkins.io/).

<!-- more -->

## The problem

When using tools like ArgoCD, you often setup a Cluster [using some kind of credentials](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters) (or kubeconfig), usually a Service Account. Normally this Service Account is granted cluster-admin permissions, which is not recommended on production environments. This is particularly problematic when teams have access to the CD tool to deploy and manage applications themselves. Leaving cluster-admin permissions allows them to modify any resource, including core Kubernetes resources: CoreDNS, Node, Secrets, etc.

## The ideal solution

The ideal solution in this scenarios is to provide the CD tools with the minimum permissions required to deploy and manage only the applications and their related resources. In general, this means creating RoleBindings on the destination namespaces allowing the creation and management of resources.

Being more specific, the desired scenario we're going to focus on is:

- A CD tool (ArgoCD for the example) is configured with a Service Account.
- The Service Account has permission to fully manage all resources in the application namespaces.
- The application namespaces are distinguished by a label, e.g. `namespace-type: application`.

!!! Note
    This is a simplified scenario and it assumes that someone (usually a cluster administrator or DevOps team) have already created the namespaces with the appropriate labels.

## The implementation

To achieve this, only one ManagedResource is really needed, which will create a RoleBinding in each namespace with the label `namespace-type: application`. The RoleBinding will grant the necessary permissions to the Service Account used by the CD tool.

!!! info
    ArgoCD could use the `in-cluster` config with its own service account, but for this example we will create a separate Service Account for the CD tool so it's extensible for external clusters too. If you want to use the in-cluster config, you can skip to point 5.

1. For simplicity's sake, we will use the `kube-system` namespace for the Service Account, but you can use any other namespace.

    ``` yaml
    apiVersion: v1
    kind: ServiceAccount
    metadata:
      name: argocd-sa
      namespace: kube-system
    ```

2. For being able to import the Service Account into ArgoCD we need to create a long-lived Secret with the Service Account token:

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: argocd-sa-token
      namespace: kube-system
      annotations:
        kubernetes.io/service-account.name: argocd-sa
    type: kubernetes.io/service-account-token
    ```

    !!! tip
        You can use short-lived tokens, but they will require to be refreshed periodically. This could be done using `ManagedResources` to update the Secret with a new token, but for simplicity we will use a long-lived token.

3. Now we can retrieve the token from the Secret and use it to configure the credentials in ArgoCD:

    ``` bash
    kubectl get secret argocd-sa-token -n kube-system -o jsonpath='{.data.token}' | base64 --decode
    ```

4. With the Service Account token register the cluster in ArgoCD:

    ``` yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: mycluster-secret
      labels:
        argocd.argoproj.io/secret-type: cluster
    type: Opaque
    stringData:
      name: mycluster.example.com
      server: https://mycluster.example.com
      config: |
        {
          "bearerToken": "<serviceaccount token>",
          "tlsClientConfig": {
            "insecure": false,
            "caData": "<base64 encoded certificate>"
          }
        }
    ```

    !!! info
        The `caData` is optional if the cluster uses a public CA or if you trust the cluster's certificate. Also you could set `insecure` to `true` if you want to skip certificate verification, but this is not recommended for production environments.

5. Once the service account is configured, we have to configure some cluster-read permissions for the Service Account so ArgoCD can populate the caches:

    ```yaml
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: argocd-read
    rules:
      - apiGroups: ["*"]
        resources: ["*"]
        verbs: ["get", "list", "watch"]
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: argocd-sa-read
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: argocd-read
    subjects:
      - kind: ServiceAccount
        name: argocd-sa
        namespace: kube-system
    ```

6. Until this point ArgoCD can connect and read resources from the cluster, but can't deploy anything. For that, only one `ManagedResource` is needed, it will create the RoleBinding in each namespace with the label `namespace-type: application` providing `edit` permissions.

    ```yaml
    apiVersion: automation.kubensync.com/v1alpha1
    kind: ManagedResource
    metadata:
      name: argocd-dynamic-rbac
    spec:
      namespaceSelector:
        labelSelector:
          matchLabels:
            namespace-type: application
      template:
        data:
          - name: argocd-sa
            type: ServiceAccount
            ref:
              name: argocd-sa
              namespace: kube-system
        literal: |
          ---
          apiVersion: rbac.authorization.k8s.io/v1
          kind: RoleBinding
          metadata:
            name: argocd-rolebinding
            namespace: {{ .Namespace.Name }}
          subjects:
            - kind: ServiceAccount
              name: argocd-sa
              namespace: kube-system
          roleRef:
            kind: ClusterRole
            name: edit
            apiGroup: rbac.authorization.k8s.io
    ```

## Conclusion

Using `ManagedResources` to dynamically configure RBAC for CD tools like ArgoCD is a viable and effective solution. It allows to automate the permission management allowing the tool to deploy only on the desired namespaces without granting cluster-wide admin permissions. This approach can be adapted to other CD tools like FluxCD or Jenkins, providing a flexible and secure way to manage permissions in Kubernetes clusters.

### Limitations

This solution assumes standard application deployments containing only basic deployments (Deployments, StatefulSets, etc.) and services. If your applications require more complex deployments, such as custom resources, cluster-scoped resources, or specific permissions, you may need to extend the `RoleBinding` or create additional `ManagedResources` to cover those cases.

This work is licensed under a [Creative Commons Attribution 4.0 International License](../../../LICENSE).
