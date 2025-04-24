set -e

NAMESPACE="test-kubensync-bugs"
MAX_WAIT=30
INTERVAL=1

echo "#59 - Resources not being deleted when namespace selected by label"

# Create a namespace for the test
kubectl create namespace $NAMESPACE

# Label the namespace to match the label selector
kubectl label namespace $NAMESPACE test-label=test-value

# Create a ManagedResource that selects the namespace based on the label
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: managedresource-sample
spec:
  namespaceSelector:
    labelSelector:
      matchLabels:
        test-label: test-value
  template:
    literal: |
      ---
      apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: test
        namespace: {{ .Namespace.Name }}
EOF

# When deleting changing the label of the namespace, the ServiceAccount should be deleted as the namespace no longer triggers the MR
kubectl label namespace $NAMESPACE test-label=test-value-2 --overwrite
valid=0

# Path the managedresource adding a label to trigger the reconciliation
kubectl patch managedresource managedresource-sample --type=merge -p '{"metadata":{"labels":{"kubensync.com/trigger":"true"}}}'
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get serviceaccount test -n $NAMESPACE > /dev/null 2>&1; then
        echo "ServiceAccount test was not deleted yet"
    else
        echo "ServiceAccount test was deleted"
        valid=1
        break
    fi
    echo "Waiting for ServiceAccount test to be deleted..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "ServiceAccount test was not deleted"
    exit 1
fi

kubectl delete managedresource managedresource-sample
kubectl delete namespace $NAMESPACE