set -e

NAMESPACE="test-kubensync-metadata"
MANAGER_NAMESPACE="kubensync-system"
MAX_WAIT=30
INTERVAL=1

kubectl create namespace "$NAMESPACE"

echo "#186 - Reconcile panics on a template document with missing or non-map metadata"

MANAGER_POD=$(kubectl get pods -n "$MANAGER_NAMESPACE" -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}')
RESTARTS_BEFORE=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.containerStatuses[0].restartCount}')

# A document with no metadata, and one whose metadata is a scalar. Both used to
# panic the reconcile (recovered, so the resource never got a status condition).
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-metadata-missing
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |
        apiVersion: v1
        kind: ConfigMap
EOF

kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-metadata-scalar
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |
        apiVersion: v1
        kind: ConfigMap
        metadata: "not-a-map"
EOF

for mr in mr-metadata-missing mr-metadata-scalar; do
    valid=0
    for (( i=0; i<MAX_WAIT; i+=INTERVAL )); do
        condition=$(kubectl get managedresource $mr -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
        if [ "$condition" = "False" ]; then
            echo "$mr has Ready=False"
            valid=1
            break
        fi
        echo "Waiting for $mr to have Ready=False..."
        sleep $INTERVAL
    done
    if [ $valid -eq 0 ]; then
        echo "Error: $mr did not reach Ready=False within $MAX_WAIT seconds (reconcile likely panicked)"
        exit 1
    fi
done

# The manager must have handled the invalid metadata without crashing.
phase=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
if [ "$phase" != "Running" ]; then
    echo "Error: manager pod $MANAGER_POD is in phase $phase, expected Running"
    exit 1
fi
RESTARTS_AFTER=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.containerStatuses[0].restartCount}')
if [ "$RESTARTS_AFTER" != "$RESTARTS_BEFORE" ]; then
    echo "Error: manager pod $MANAGER_POD restarted while handling invalid metadata ($RESTARTS_BEFORE -> $RESTARTS_AFTER)"
    exit 1
fi
echo "Manager pod $MANAGER_POD is running with $RESTARTS_AFTER restarts"

kubectl delete managedresource mr-metadata-missing mr-metadata-scalar
kubectl delete namespace "$NAMESPACE"
