#!/bin/bash

set -e

echo "#############################################"
echo "#                                           #"
echo "#        Running LabelSelector Tests        #"
echo "#                                           #"
echo "#############################################"

NAMESPACE="test-kubensync-labelselector"
MAX_WAIT=30
INTERVAL=1

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

kubectl delete namespace $NAMESPACE
kubectl delete managedresource managedresource-sample
