# KubeNSync operator
Introducing KubeNSync, a Kubernetes operator designed to simplify the creation of repetitive resources within distinct namespaces (but also cluster wide!). Craft resource templates tailored to your needs, and with the flexibility of a namespace selector, effortlessly synchronize chosen resources. Streamline resource management and enhance deployment efficiency using KubeNSync's intuitive approach.

# Description
KubeNSync is a Kubernetes operator that helps you automate the creation of Kubernetes resources. You can use it to create resources like pull secrets, RBAC rules, and operators using Go templates. With KubeNSync, you can define a Custom Resource (CR) that contains the template to be rendered and a namespace selector in the form of a or label selectors. This allows you to create and manage resources in specific namespaces or across the entire cluster.
