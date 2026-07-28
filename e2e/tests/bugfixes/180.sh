set -e

NAMESPACE="test-kubensync-partial"
MAX_WAIT=30
INTERVAL=1

kubectl create namespace "$NAMESPACE"

echo "#180 - Partially applied resources are missing from status.createdResources"

# First document is valid and applies; the second has a name the API server
# rejects, so the apply loop fails after creating the first.
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-partial-apply
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ConfigMap
        metadata:
            name: partial-ok
            namespace: {{ .Namespace.Name }}
        ---
        apiVersion: v1
        kind: ConfigMap
        metadata:
            name: Invalid_Name_Here
            namespace: {{ .Namespace.Name }}
EOF

valid=0
for (( i=0; i<MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-partial-apply -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-partial-apply has Ready=False"
        valid=1
        break
    fi
    echo "Waiting for mr-partial-apply to have Ready=False..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "Error: mr-partial-apply did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

# The valid first document was applied.
if ! kubectl get configmap partial-ok -n "$NAMESPACE" > /dev/null 2>&1; then
    echo "Error: ConfigMap partial-ok was not created"
    exit 1
fi
echo "ConfigMap partial-ok was created"

# The applied resource must be recorded in the status, even though the pass failed.
tracked=$(kubectl get managedresource mr-partial-apply -o jsonpath='{.status.createdResources[?(@.name=="partial-ok")].name}' 2>/dev/null || true)
if [ "$tracked" != "partial-ok" ]; then
    echo "Error: partial-ok is not tracked in .status.createdResources"
    echo "createdResources: $(kubectl get managedresource mr-partial-apply -o jsonpath='{.status.createdResources}')"
    exit 1
fi
echo "partial-ok is tracked in .status.createdResources"

kubectl delete managedresource mr-partial-apply
kubectl delete namespace "$NAMESPACE"
