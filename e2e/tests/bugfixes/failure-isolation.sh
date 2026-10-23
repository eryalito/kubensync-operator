set -e

NAMESPACE="test-kubensync-iso"
NS_PREFIX="test-kubensync-iso-ns"
MAX_WAIT=30
INTERVAL=1

echo "A broken ManagedResource or namespace does not block the reconciles that should work"

wait_for_configmap() {
    for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
        if kubectl get configmap "$2" -n "$1" > /dev/null 2>&1; then
            echo "ConfigMap $2 exists in $1"
            return 0
        fi
        sleep $INTERVAL
    done
    echo "Error: ConfigMap $2 was not created in $1"
    exit 1
}

wait_for_configmap_deleted() {
    for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
        if ! kubectl get configmap "$2" -n "$1" > /dev/null 2>&1; then
            echo "ConfigMap $2 was deleted from $1"
            return 0
        fi
        sleep $INTERVAL
    done
    echo "Error: ConfigMap $2 was not deleted from $1"
    exit 1
}

wait_for_ready_false() {
    for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
        condition=$(kubectl get managedresource "$1" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
        if [ "$condition" == "False" ]; then
            echo "$1 has Ready=False: $(kubectl get managedresource "$1" -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}')"
            return 0
        fi
        sleep $INTERVAL
    done
    echo "Error: $1 did not reach Ready=False"
    exit 1
}

# create_mr NAME SELECTOR CONFIGMAP VALUE
create_mr() {
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: $1
spec:
  namespaceSelector:
$2
  template:
    literal: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: $3
        namespace: {{ .Namespace.Name }}
      data:
        value: $4
EOF
}

echo "Namespace events: a ManagedResource that always fails, sorted before a working one"
LABEL_SELECTOR="    labelSelector:
      matchLabels:
        kubensync-iso: \"true\""
create_mr mr-iso-a-broken "$LABEL_SELECTOR" iso-a "{{ .Namespace.Nope }}"
create_mr mr-iso-b-ok "$LABEL_SELECTOR" iso-b ok

kubectl create namespace $NAMESPACE
kubectl label namespace $NAMESPACE kubensync-iso=true
wait_for_configmap $NAMESPACE iso-b
wait_for_ready_false mr-iso-a-broken

kubectl label namespace $NAMESPACE kubensync-iso-
wait_for_configmap_deleted $NAMESPACE iso-b

echo "Full pass: a namespace that fails to render, sorted before a working one"
kubectl create namespace $NS_PREFIX-a
kubectl create namespace $NS_PREFIX-b
create_mr mr-iso-ns "    regex: \"^$NS_PREFIX-\"" iso-ns "{{ if eq .Namespace.Name \"$NS_PREFIX-a\" }}{{ .Namespace.Nope }}{{ else }}ok{{ end }}"
wait_for_configmap $NS_PREFIX-b iso-ns
wait_for_ready_false mr-iso-ns

kubectl delete managedresource mr-iso-a-broken mr-iso-b-ok mr-iso-ns
kubectl delete namespace $NAMESPACE $NS_PREFIX-a $NS_PREFIX-b
