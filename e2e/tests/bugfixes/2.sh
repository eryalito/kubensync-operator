set -e

NAMESPACE="test-kubensync-bugs"
MAX_WAIT=30
INTERVAL=1

kubectl create namespace "$NAMESPACE"

echo "#2 - Resources outside namespace are not deleted"

## OUTSIDE NAMESPACE 
# Create a SA in the kube-system namespace for the $NAMESPACE namespace
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
                name: "$NAMESPACE-sa"
                namespace: kube-system
EOF

# When deleting the original namespace, the SA should be deleted as the trigger namespace is deleted
kubectl delete namespace "$NAMESPACE"
valid=0

# Path the managedresource adding a label to trigger the reconciliation
kubectl patch managedresource managedresource-sample --type=merge -p '{"metadata":{"labels":{"kubensync.com/trigger":"true"}}}'
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get serviceaccount "$NAMESPACE-sa" -n kube-system > /dev/null 2>&1; then
        echo "ServiceAccount $NAMESPACE-sa was not deleted yet"
    else
        echo "ServiceAccount $NAMESPACE-sa was deleted"
        valid=1
        break
    fi
    echo "Waiting for ServiceAccount $NAMESPACE-sa to be deleted..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "ServiceAccount $NAMESPACE-sa was not deleted"
    exit 1
fi

kubectl delete managedresource managedresource-sample

## CLUSTER WIDE OBJECT
kubectl create namespace "$NAMESPACE"
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
            apiVersion: rbac.authorization.k8s.io/v1
            kind: ClusterRoleBinding
            metadata:
                name: $NAMESPACE
            roleRef:
                apiGroup: rbac.authorization.k8s.io
                kind: ClusterRole
                name: system:basic-user
            subjects:
              - apiGroup: rbac.authorization.k8s.io
                kind: Group
                name: system:authenticated

EOF

# When deleting the original namespace, the SA should be deleted as the trigger namespace is deleted
kubectl delete namespace "$NAMESPACE"
valid=0

# Path the managedresource adding a label to trigger the reconciliation
kubectl patch managedresource managedresource-sample --type=merge -p '{"metadata":{"labels":{"kubensync.com/trigger":"true"}}}'
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get clusterrolebinding "$NAMESPACE" > /dev/null 2>&1; then
        echo "ClusterRoleBinding $NAMESPACE was not deleted yet"
    else
        echo "ClusterRoleBinding $NAMESPACE was deleted"
        valid=1
        break
    fi
    echo "Waiting for ClusterRoleBinding $NAMESPACE to be deleted..."
    sleep $INTERVAL
done

if [ $valid -eq 0 ]; then
    echo "ClusterRoleBinding $NAMESPACE was not deleted"
    exit 1
fi

kubectl delete managedresource managedresource-sample