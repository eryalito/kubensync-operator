set -e

NAMESPACE="test-kubensync-malformed"
CANARY_NAMESPACE="test-kubensync-malformed-canary"
MANAGER_NAMESPACE="kubensync-system"
MARKER="leak-marker-do-not-echo-abc123"
MAX_WAIT=30
INTERVAL=1

kubectl create namespace "$NAMESPACE"

echo "Malformed template documents must not wedge the reconciler"

MANAGER_POD=$(kubectl get pods -n "$MANAGER_NAMESPACE" -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}')
RESTARTS_BEFORE=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.containerStatuses[0].restartCount}')
echo "Manager pod $MANAGER_POD has $RESTARTS_BEFORE restarts before the test"

# A healthy ManagedResource that no namespace matches yet, reconciled later by a
# namespace event.
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-malformed-canary
spec:
  namespaceSelector:
    regex: "^$CANARY_NAMESPACE\$"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ConfigMap
        metadata:
            name: malformed-canary
            namespace: {{ .Namespace.Name }}
EOF

# An unambiguous JSON stream whose last document has a syntax error. The decoder
# returns that error from every later Decode without consuming input, so skipping
# the document and decoding again never terminates.
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-malformed-json-stream
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |-
        {"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"malformed-stream-1","namespace":"{{ .Namespace.Name }}"}}
        {"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"malformed-stream-2","namespace":"{{ .Namespace.Name }}"}}
        {"$MARKER":,}
EOF

valid=0
for (( i=0; i<MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-malformed-json-stream -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-malformed-json-stream has Ready=False"
        valid=1
        break
    fi
    echo "Waiting for mr-malformed-json-stream to have Ready=False..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "Error: mr-malformed-json-stream did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

# The whole template is decoded before anything is applied.
for name in malformed-stream-1 malformed-stream-2; do
    if kubectl get configmap "$name" -n "$NAMESPACE" > /dev/null 2>&1; then
        echo "Error: ConfigMap $name was created even though the template holds a malformed document"
        exit 1
    fi
done
echo "No resource of mr-malformed-json-stream was applied"

# The status is readable by anyone holding the viewer role, so it must not echo
# the rendered template.
status=$(kubectl get managedresource mr-malformed-json-stream -o jsonpath='{.status}' 2>/dev/null || true)
if echo "$status" | grep -q "$MARKER"; then
    echo "Error: the status of mr-malformed-json-stream echoes the rendered template: $status"
    exit 1
fi
echo "The status of mr-malformed-json-stream does not echo the rendered template"

# A single JSON document followed by fewer than 4 bytes of invalid data. The
# decoder cannot fall back to YAML and reaches a different terminal state than
# the stream above.
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-malformed-json-tail
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |-
        {"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"malformed-tail-1","namespace":"{{ .Namespace.Name }}"}} x
EOF

valid=0
for (( i=0; i<MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-malformed-json-tail -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-malformed-json-tail has Ready=False"
        valid=1
        break
    fi
    echo "Waiting for mr-malformed-json-tail to have Ready=False..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "Error: mr-malformed-json-tail did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

# A YAML stream advances past a malformed document, so it never stalls the
# decoder. The bad document used to be skipped, applying the good ones and still
# reporting the resource as ready.
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-malformed-yaml
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ConfigMap
        metadata:
            name: malformed-yaml-1
            namespace: {{ .Namespace.Name }}
        ---
        apiVersion: v1
        kind: "ConfigMap
        metadata:
            name: malformed-yaml-2
            namespace: {{ .Namespace.Name }}
EOF

valid=0
for (( i=0; i<MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-malformed-yaml -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-malformed-yaml has Ready=False"
        valid=1
        break
    fi
    echo "Waiting for mr-malformed-yaml to have Ready=False..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "Error: mr-malformed-yaml did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

if kubectl get configmap malformed-yaml-1 -n "$NAMESPACE" > /dev/null 2>&1; then
    echo "Error: ConfigMap malformed-yaml-1 was created even though a later document is malformed"
    exit 1
fi
echo "No resource of mr-malformed-yaml was applied"

# All three malformed ManagedResources stay in place: if any of them wedged the
# shared reconciler lock, this namespace event would never be served.
kubectl create namespace "$CANARY_NAMESPACE"

valid=0
for (( i=0; i<MAX_WAIT; i+=INTERVAL )); do
    if kubectl get configmap malformed-canary -n "$CANARY_NAMESPACE" > /dev/null 2>&1; then
        echo "ConfigMap malformed-canary was created while the malformed ManagedResources exist"
        valid=1
        break
    fi
    echo "Waiting for ConfigMap malformed-canary to be created..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "Error: the operator stopped reconciling: malformed-canary was not created within $MAX_WAIT seconds"
    exit 1
fi

# The manager must have handled the malformed templates without crashing.
phase=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
if [ "$phase" != "Running" ]; then
    echo "Error: manager pod $MANAGER_POD is in phase $phase, expected Running"
    exit 1
fi

RESTARTS_AFTER=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.containerStatuses[0].restartCount}')
if [ "$RESTARTS_AFTER" != "$RESTARTS_BEFORE" ]; then
    echo "Error: manager pod $MANAGER_POD restarted while handling the malformed templates ($RESTARTS_BEFORE -> $RESTARTS_AFTER)"
    exit 1
fi
echo "Manager pod $MANAGER_POD is running with $RESTARTS_AFTER restarts"

kubectl delete managedresource mr-malformed-json-stream
kubectl delete managedresource mr-malformed-json-tail
kubectl delete managedresource mr-malformed-yaml
kubectl delete managedresource mr-malformed-canary
kubectl delete namespace "$CANARY_NAMESPACE"
kubectl delete namespace "$NAMESPACE"
