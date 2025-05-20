# Namespace Selector

Namespaces can be selected using a regex or a label selector. The regex is applied to the namespace name, while the label selector is applied to the namespace labels. The regex and label selector are combined using an AND operation.

## Regex

The regex is applied to the namespace name. The regex is applied directly to the namespace name, so it looks for matches in the namespace name itself. For example, the regex `test` will match any namespace that contains the string `test` in its name, such as `test`, `test-1`, or `my-test-namespace`.

The regex is a standard Go regex, so you can use any valid Go regex syntax. For example, the regex `^test-.*` will match any namespace that starts with `test-`, such as `test-1`, `test-2`, or `test-abc`.

!!! tip
    As the regex is matching anything in the namespace name, it is recommended to use a regex that is as specific as possible. For example, if you want to match only namespaces that start with `test-`, you should use the regex `^test-.*` instead of just `test`. This will help avoid matching unintended namespaces.

## Label Selector

The label selector is applied to the namespace labels. The label selector is a standard Kubernetes label selector. For example, the label selector `environment=production` will match any namespace that has the label `environment` set to `production`.

## Example

Selecting all namespaces that start with `shopping-` and have the label `environment=production`:

```yaml
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    avoidResourceUpdate: false
    namespaceSelector:
        regex: "^shopping-.*"
        labelSelector:
            matchLabels:
                environment: production
    template:
        # ...
        # Additional YAML configuration goes here
        # ...
```
