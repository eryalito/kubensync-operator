# Usage

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
        labelSelector: # (3)!
          matchLabels: {} # (4)!
          matchExpressions: {} # (5)!
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
                .dockerconfigjson: '{{ index .Data.pull_secret ".dockerconfigjson" | base64Encode }}'
```

1. !!! tip
    You can read as many secrets or configmaps as you need, even if they are duplicates. Just keep in mind that name should be unique.

2. !!! info
    This will be the value used on the template

3. !!! info
   Select namespaces based on labels.

4. !!! info

    ``` text
    DESCRIPTION:
      matchLabels is a map of {key,value} pairs. A single {key,value} in the
      matchLabels map is equivalent to an element of matchExpressions, whose key
      field is "key", the operator is "In", and the values array contains only
      "value". The requirements are ANDed.
    ```

5. !!! info

    ``` text
    DESCRIPTION:
      matchExpressions is a list of label selector requirements. The requirements
      are ANDed.

      A label selector requirement is a selector that contains values, a key, and
      an operator that relates the key and values.

    FIELDS:
      key  <string> -required-
        key is the label key that the selector applies to.

      operator     <string> -required-
        operator represents a key's relationship to a set of values. Valid
        operators are In, NotIn, Exists and DoesNotExist.

      values       <[]string>
        values is an array of string values. If the operator is In or NotIn, the
        values array must be non-empty. If the operator is Exists or DoesNotExist,
        the values array must be empty. This array is replaced during a strategic
        merge patch.
    ```

!!! question
    - `avoidResourceUpdate`: Optional field that changes the default behavior of reconciling existing resources with the desired state. If set to true only non-existing resources will be created an never updated. Default values is `false`.
    - `namespaceSelector`: Specifies the namespaces where you want to apply the template. You can use a regular expression (regex) to match multiple namespaces or filter them by its labels. Regex and labels are ANDed, the namespaces must match both of them to be selected. If none of them are defined, all namespaces will be selected.
    - `template`: Contains the YAML template that you want to apply to the selected namespaces. You can use Go template syntax to customize the resource based on the namespace.
    - `template.data`: Optional field that read Kubernetes resources and expose their contents to be used in the `template` under `.Data.<name>`.

## Examples

Check out some real-world use cases of kubensync in the [examples](../examples/index.md) section.
