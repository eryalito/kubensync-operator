set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests for cluster         #"
echo "#    scoped resources...                    #"
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
        apiVersion: rbac.authorization.k8s.io/v1
        kind: ClusterRole
        metadata:
            name: test-clusterrole-{{ .Namespace.Name }}
        rules:
        - apiGroups: [""]
          resources: ["pods"]
          verbs: ["get", "watch", "list"]
EOF

exists=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get clusterrole "test-clusterrole-$NAMESPACE" > /dev/null 2>&1; then
        echo "ClusterRole test-clusterrole-$NAMESPACE created"
        exists=1
        break
    fi
    echo "Waiting for ClusterRole test-clusterrole-$NAMESPACE to be created..."
    sleep $INTERVAL
done

if [ $exists -eq 0 ]; then
    echo "ClusterRole test-clusterrole-$NAMESPACE was not created"
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
        apiVersion: rbac.authorization.k8s.io/v1
        kind: ClusterRole
        metadata:
            name: test-clusterrole-{{ .Namespace.Name }}
        rules:
        - apiGroups: [""]
          resources: ["pods"]
          verbs: ["get", "watch", "list"]
        ---
        apiVersion: rbac.authorization.k8s.io/v1
        kind: ClusterRole
        metadata:
            name: test-clusterrole2-{{ .Namespace.Name }}
        rules:
        - apiGroups: [""]
          resources: ["pods"]
          verbs: ["get", "watch", "list"]
EOF


exists=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get clusterrole "test-clusterrole2-$NAMESPACE" > /dev/null 2>&1; then
        echo "ClusterRole test-clusterrole2-$NAMESPACE created"
        exists=1
        break
    fi
    echo "Waiting for ClusterRole test-clusterrole2-$NAMESPACE to be created..."
    sleep $INTERVAL
done

if [ $exists -eq 0 ]; then
    echo "ClusterRole test-clusterrole2-$NAMESPACE was not created"
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
        apiVersion: rbac.authorization.k8s.io/v1
        kind: ClusterRole
        metadata:
            name: test-clusterrole2-{{ .Namespace.Name }}
        rules:
        - apiGroups: [""]
          resources: ["pods"]
          verbs: ["get", "watch", "list"]
EOF

for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
  if ! kubectl get clusterrole "test-clusterrole-$NAMESPACE" > /dev/null 2>&1; then
    echo "ClusterRole test-clusterrole-$NAMESPACE has been deleted"
    break
  fi
  echo "Waiting for ClusterRole test-clusterrole-$NAMESPACE to be deleted..."
  sleep $INTERVAL
done

if (( i == MAX_WAIT )); then
  echo "Error: ClusterRole test-clusterrole-$NAMESPACE was not deleted within $MAX_WAIT seconds"
  exit 1
fi

kubectl delete managedresource managedresource-sample
