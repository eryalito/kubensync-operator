set -e

echo "#############################################"
echo "#                                           #"
echo "#   Running Data Kubernetes Resource Tests  #"
echo "#                                           #"
echo "#############################################"

NAMESPACE="test-kubensync"
MAX_WAIT=30
INTERVAL=1


# test cm injection
kubectl create -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
    name: my-configmap
    namespace: $NAMESPACE
data:
    special-key: "checkable-value"
EOF


kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
    name: managedresource-sample
spec:
    namespaceSelector:
        regex: "$NAMESPACE"
    template:
        data:
          - name: cm
            type: KubernetesResource
            ref:
                name: my-configmap
                namespace: "$NAMESPACE"
                apiVersion: v1
                kind: ConfigMap
                group: ""
        literal: |
            ---
            apiVersion: v1
            kind: Secret
            metadata:
                name: test-secret
                namespace: {{ .Namespace.Name }}
            stringData:
                value: "{{ index .Data.cm.data "special-key" }}"
        
EOF

exists=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get secret "test-secret" -n "$NAMESPACE" > /dev/null 2>&1; then
        echo "Secret test-secret created in namespace $NAMESPACE"
        exists=1
        break
    fi
    echo "Waiting for Secret test-secret to be created in namespace $NAMESPACE..."
    sleep $INTERVAL
done

if [ $exists -eq 0 ]; then
    echo "Secret test-secret was not created in namespace $NAMESPACE"
    exit 1
fi

secret_value=$(kubectl get secret "test-secret" -n "$NAMESPACE" -o jsonpath='{.data.value}' | base64 --decode)
if [ "$secret_value" != "checkable-value" ]; then
    echo "Secret test-secret does not contain the expected value"
    exit 1
else
    echo "Secret test-secret contains the expected value '$secret_value'"
fi

kubectl delete cm my-configmap -n $NAMESPACE
kubectl delete managedresource managedresource-sample
