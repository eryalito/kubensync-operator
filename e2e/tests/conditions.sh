set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting conditions e2e tests          #"
echo "#                                           #"
echo "#############################################"

NAMESPACE="test-kubensync-conditions"
MR_NAME="managedresource-conditions"
MAX_WAIT=40
INTERVAL=2

# Create isolated namespace used by the MR selector
kubectl create namespace "$NAMESPACE" >/dev/null 2>&1 || true

echo "Creating ManagedResource with a valid template (should become Ready=True)"
kubectl apply -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: ${MR_NAME}
spec:
  namespaceSelector:
    regex: "^${NAMESPACE}$"
  template:
    literal: |
      ---
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: conditions-test
        namespace: {{ .Namespace.Name }}
      data:
        phase: initial
EOF

# Wait for resource creation
created=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
  if kubectl get configmap conditions-test -n "$NAMESPACE" >/dev/null 2>&1; then
    created=1
    break
  fi
  echo "Waiting for ConfigMap conditions-test to be created..."
  sleep $INTERVAL
done

if [ $created -eq 0 ]; then
  echo "ConfigMap conditions-test not created"
  kubectl get managedresource ${MR_NAME} -o yaml || true
  exit 1
fi

# Wait for Ready=True condition
ready=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
  status=$(kubectl get managedresource ${MR_NAME} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
  if [ "$status" == "True" ]; then
    echo "ManagedResource ${MR_NAME} is Ready=True"
    ready=1
    break
  fi
  echo "Waiting for ManagedResource ${MR_NAME} Ready condition... current: '$status'"
  sleep $INTERVAL
done

if [ $ready -eq 0 ]; then
  echo "ManagedResource ${MR_NAME} did not become Ready=True"
  kubectl get managedresource ${MR_NAME} -o yaml || true
  exit 1
fi

echo "Updating ManagedResource with an invalid template (should become Ready=False)"
kubectl apply -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: ${MR_NAME}
spec:
  namespaceSelector:
    regex: "^${NAMESPACE}$"
  template:
    literal: |
      ---
      apiVersion: v1
      kind: NonExistingKind
      metadata:
        name: invalid-object
        namespace: {{ .Namespace.Name }}
EOF

# Wait for Ready=False condition
notready=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
  status=$(kubectl get managedresource ${MR_NAME} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
  if [ "$status" == "False" ]; then
    echo "ManagedResource ${MR_NAME} is Ready=False after invalid update"
    notready=1
    break
  fi
  echo "Waiting for ManagedResource ${MR_NAME} Ready=False condition... current: '$status'"
  sleep $INTERVAL
done

if [ $notready -eq 0 ]; then
  echo "ManagedResource ${MR_NAME} did not become Ready=False"
  kubectl get managedresource ${MR_NAME} -o yaml || true
  exit 1
fi

echo "Conditions test passed"

# Cleanup
kubectl delete managedresource ${MR_NAME} --ignore-not-found
kubectl delete namespace ${NAMESPACE} --ignore-not-found
