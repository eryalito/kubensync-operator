
Once the kubensync operator is installed, you can start using it by defining custom resources (CRs) that specify the resources you want to synchronize.

## ManagedResource

The ManagedResource kind allows users to define a template to apply for each selected namespace. 

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
        data: # (1)!
        - name: pull_secret # (2)!
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

1.  !!! tip 
    You can read as many secrets or configmaps as you need, even if they are duplicates. Just keep in mind that name should be unique.

2.  !!! info 
    This will be the value used on the template

!!! question 
    - `avoidResourceUpdate`: Optional field that changes the default behavior of reconciling existing resources with the desired state. If set to true only non-existing resources will be created an never updated. Default values is `false`.
    - `namespaceSelector`: Specifies the namespaces where you want to apply the template. You can use a regular expression (regex) to match multiple namespaces.
    - `template`: Contains the YAML template that you want to apply to the selected namespaces. You can use Go template syntax to customize the resource based on the namespace.
    - `template.data`: Optional field that read `Secret` or `ConfigMap` and imports the contents to be used in the `template` under `.Data.<name>`.


