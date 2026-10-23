set -e

NAMESPACE="test-kubensync-ready-recovery"
MAX_WAIT=60
INTERVAL=1

echo "Ready recovers after a failure reported by the namespace controller, without touching the ManagedResource"

ready_status() {
    kubectl get managedresource mr-ready-recovery -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true
}

wait_for_ready() {
    for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
        if [ "$(ready_status)" == "$1" ]; then
            echo "mr-ready-recovery has Ready=$1"
            return 0
        fi
        sleep $INTERVAL
    done
    echo "Error: mr-ready-recovery has Ready=$(ready_status), expected $1"
    exit 1
}

# The template fails for the namespace until it gets the label fixed=true
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-ready-recovery
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ready-recovery
        namespace: {{ .Namespace.Name }}
      data:
        value: {{ if index .Namespace.Labels "fixed" }}ok{{ else }}{{ .Namespace.Nope }}{{ end }}
EOF
wait_for_ready True

kubectl create namespace $NAMESPACE
wait_for_ready False

kubectl label namespace $NAMESPACE fixed=true
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get configmap ready-recovery -n $NAMESPACE > /dev/null 2>&1; then
        echo "ConfigMap ready-recovery created"
        break
    fi
    sleep $INTERVAL
done
wait_for_ready True

kubectl delete managedresource mr-ready-recovery
kubectl delete namespace $NAMESPACE
