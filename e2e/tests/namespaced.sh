set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests for namespaced      #"
echo "#    resources...                           #"
echo "#                                           #"
echo "#############################################"

NAMESPACE="test-kubensync"
MAX_WAIT=30
INTERVAL=1

# Basic MR that creates a SA on the namespace
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: managedresource-sample
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test
            namespace: {{ .Namespace.Name }}
EOF

exists=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get serviceaccount "test" -n "$NAMESPACE" > /dev/null 2>&1; then
        echo "ServiceAccount test created in namespace $NAMESPACE"
        exists=1
        break
    fi
    echo "Waiting for ServiceAccount test to be created in namespace $NAMESPACE..."
    sleep $INTERVAL
done

if [ $exists -eq 0 ]; then
    echo "ServiceAccount test was not created in namespace $NAMESPACE"
    exit 1
fi

kubectl apply -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: managedresource-sample
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test
            namespace: {{ .Namespace.Name }}
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test2
            namespace: {{ .Namespace.Name }}
EOF


exists=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get serviceaccount "test2" -n "$NAMESPACE" > /dev/null 2>&1; then
        echo "ServiceAccount test2 created in namespace $NAMESPACE"
        exists=1
        break
    fi
    echo "Waiting for ServiceAccount test2 to be created in namespace $NAMESPACE..."
    sleep $INTERVAL
done

if [ $exists -eq 0 ]; then
    echo "ServiceAccount test2 was not created in namespace $NAMESPACE"
    exit 1
fi

kubectl apply -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: managedresource-sample
spec:
  namespaceSelector:
    regex: "$NAMESPACE"
  template:
    literal: |
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
            name: test2
            namespace: {{ .Namespace.Name }}
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
  if ! kubectl get serviceaccount "test" -n "$NAMESPACE" > /dev/null 2>&1; then
    echo "ServiceAccount test has been deleted from namespace $NAMESPACE"
    break
  fi
  echo "Waiting for ServiceAccount test to be deleted from namespace $NAMESPACE..."
  sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
  echo "Error: ServiceAccount test was not deleted from namespace $NAMESPACE within $MAX_WAIT seconds"
  exit 1
fi

kubectl delete managedresource managedresource-sample
