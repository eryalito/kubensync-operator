# kubensync-operator

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.4.0](https://img.shields.io/badge/AppVersion-v0.4.0-informational?style=flat-square)

A Helm chart for installing the kubensync-operator.

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| eryalito | <eryalito@gmail.com> | <https://eryalito.dev> |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | This is for setting the affinity for the operator. |
| fullnameOverride | string | `""` | This is to override the chart fullname. |
| image.pullPolicy | string | `"IfNotPresent"` | This sets the pull policy for images. |
| image.repository | string | `"ghcr.io/eryalito/kubensync-operator"` | Repository |
| image.tag | string | `""` | Overrides the image tag whose default is the chart appVersion. |
| imagePullSecrets | list | `[]` | This is for the secretes for pulling an image from a private repository more information can be found here: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/ |
| livenessProbe.httpGet.initialDelaySeconds | int | `15` |  |
| livenessProbe.httpGet.path | string | `"/healthz"` |  |
| livenessProbe.httpGet.periodSeconds | int | `20` |  |
| livenessProbe.httpGet.port | int | `8081` |  |
| nameOverride | string | `""` | This is to override the chart name. |
| nodeSelector | object | `{}` | This is for setting the nodeSelector for the operator. |
| podAnnotations | object | `{}` | This is for setting Kubernetes Annotations to a Pod. |
| podLabels | object | `{}` | This is for setting Kubernetes Labels to a Pod. |
| podSecurityContext | object | `{}` | Security context for the pod. |
| readinessProbe.httpGet.path | string | `"/readyz"` |  |
| readinessProbe.httpGet.port | int | `8081` |  |
| readinessProbe.initialDelaySeconds | int | `5` |  |
| readinessProbe.periodSeconds | int | `10` |  |
| replicaCount | int | `1` | Number of replicas |
| resources | object | `{}` | Resources to request and limit for the operator. |
| securityContext | object | `{}` | Security context for the container. |
| service.port | int | `80` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| tolerations | list | `[]` | This is for setting the tolerations for the operator. |
| volumeMounts | list | `[]` |  |
| volumes | list | `[]` |  |
