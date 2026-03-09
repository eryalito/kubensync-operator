set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests for status          #"
echo "#    conditions...                          #"
echo "#                                           #"
echo "#############################################"

NAMESPACE="test-kubensync"
MAX_WAIT=30
INTERVAL=1

# Test 1: Successful sync → Ready=True
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-ready
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test-condition-ready
            namespace: {{ .Namespace.Name }}
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-ready -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "True" ]; then
        echo "mr-condition-ready has Ready=True"
        break
    fi
    echo "Waiting for mr-condition-ready to have Ready=True..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-ready did not reach Ready=True within $MAX_WAIT seconds"
    exit 1
fi

kubectl delete managedresource mr-condition-ready

# Test 2: Invalid template syntax → Ready=False
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-bad-template
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        {{
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-bad-template -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-condition-bad-template has Ready=False"
        break
    fi
    echo "Waiting for mr-condition-bad-template to have Ready=False..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-bad-template did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

kubectl delete managedresource mr-condition-bad-template

# Test 3: Template renders valid YAML but an unknown Kubernetes kind → Ready=False
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-bad-resource
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: NonExistentKind
        metadata:
            name: test-condition-bad-resource
            namespace: {{ .Namespace.Name }}
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-bad-resource -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-condition-bad-resource has Ready=False"
        break
    fi
    echo "Waiting for mr-condition-bad-resource to have Ready=False..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-bad-resource did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

kubectl delete managedresource mr-condition-bad-resource

# Test 4: Recovery — Ready goes from False back to True after fixing the template
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-recovery
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        {{
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-recovery -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-condition-recovery has Ready=False (before fix)"
        break
    fi
    echo "Waiting for mr-condition-recovery to have Ready=False..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-recovery did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

kubectl apply -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-recovery
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test-condition-recovery
            namespace: {{ .Namespace.Name }}
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-recovery -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "True" ]; then
        echo "mr-condition-recovery has Ready=True (after fix)"
        break
    fi
    echo "Waiting for mr-condition-recovery to recover to Ready=True..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-recovery did not recover to Ready=True within $MAX_WAIT seconds"
    exit 1
fi

kubectl delete managedresource mr-condition-recovery

# Test 5: No matching namespaces → Ready=True (vacuous success)
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-no-match
spec:
  namespaceSelector:
    regex: "^this-namespace-does-not-exist-12345$"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test-condition-no-match
            namespace: {{ .Namespace.Name }}
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-no-match -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "True" ]; then
        echo "mr-condition-no-match has Ready=True (no namespaces matched)"
        break
    fi
    echo "Waiting for mr-condition-no-match to have Ready=True..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-no-match did not reach Ready=True within $MAX_WAIT seconds"
    exit 1
fi

kubectl delete managedresource mr-condition-no-match

# Test 6: Ready=False condition message is non-empty
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: mr-condition-message
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        {{
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    condition=$(kubectl get managedresource mr-condition-message -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$condition" = "False" ]; then
        echo "mr-condition-message has Ready=False"
        break
    fi
    echo "Waiting for mr-condition-message to have Ready=False..."
    sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
    echo "Error: mr-condition-message did not reach Ready=False within $MAX_WAIT seconds"
    exit 1
fi

message=$(kubectl get managedresource mr-condition-message -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}' 2>/dev/null || true)
if [ -z "$message" ]; then
    echo "Error: Ready=False condition has an empty message"
    exit 1
fi
echo "Ready=False condition message: $message"

kubectl delete managedresource mr-condition-message
