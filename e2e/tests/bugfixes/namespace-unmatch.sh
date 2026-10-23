set -e

NS_A="test-kubensync-unmatch-a"
NS_B="test-kubensync-unmatch-b"
MAX_WAIT=30
INTERVAL=1

echo "Resources are deleted as soon as their namespace stops matching, without reconciling the ManagedResource"

# create_mr NAME LABEL VALUE SERVICEACCOUNT
create_mr() {
kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: $1
spec:
  namespaceSelector:
    labelSelector:
      matchLabels:
        $2: $3
  template:
    literal: |
      ---
      apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: $4
        namespace: {{ .Namespace.Name }}
EOF
}

sa_exists() {
    kubectl get serviceaccount "$1" -n "$2" > /dev/null 2>&1
}

is_ready() {
    [ "$(kubectl get managedresource "$1" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)" == "True" ]
}

tracked_namespaces() {
    kubectl get managedresource "$1" -o jsonpath='{.status.createdResources[*].triggerNamespace}' | tr ' ' '\n' | sort | xargs
}

others_intact() {
    sa_exists test-unmatch "$NS_B" && sa_exists test-unmatch-other "$NS_A" && sa_exists test-unmatch-other "$NS_B" \
        && [ "$(tracked_namespaces mr-namespace-unmatch-other)" == "$NS_A $NS_B" ]
}

for ns in $NS_A $NS_B; do
    kubectl create namespace $ns
    kubectl label namespace $ns test-label=test-value other-label=other-value
done

# Both ManagedResources select both namespaces, each through its own label
create_mr mr-namespace-unmatch test-label test-value test-unmatch
create_mr mr-namespace-unmatch-other other-label other-value test-unmatch-other

# Wait for both ManagedResource reconciles to finish, so the deletion below can only come from the namespace event
ready=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if is_ready mr-namespace-unmatch && is_ready mr-namespace-unmatch-other \
        && sa_exists test-unmatch "$NS_A" && others_intact; then
        echo "All ServiceAccounts created and both ManagedResources are Ready"
        ready=1
        break
    fi
    echo "Waiting for the ServiceAccounts and both ManagedResources to be Ready..."
    sleep $INTERVAL
done

if [ $ready -eq 0 ]; then
    echo "The ServiceAccounts were not created or the ManagedResources did not become Ready"
    exit 1
fi

# Only mr-namespace-unmatch stops matching, and only in $NS_A
kubectl label namespace $NS_A test-label-

removed=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if ! sa_exists test-unmatch "$NS_A" && [ "$(tracked_namespaces mr-namespace-unmatch)" == "$NS_B" ]; then
        echo "ServiceAccount test-unmatch was deleted from $NS_A and is no longer tracked"
        removed=1
        break
    fi
    echo "Waiting for ServiceAccount test-unmatch to be deleted from $NS_A and untracked..."
    sleep $INTERVAL
done

if [ $removed -eq 0 ]; then
    echo "ServiceAccount test-unmatch was not deleted from $NS_A or is still tracked: $(tracked_namespaces mr-namespace-unmatch)"
    exit 1
fi

# The other namespace and the other ManagedResource must keep their resources
for (( i=0; i<5; i+=INTERVAL )); do
    if ! others_intact || [ "$(tracked_namespaces mr-namespace-unmatch)" != "$NS_B" ]; then
        echo "A resource that still matches was deleted or untracked"
        kubectl get serviceaccount -n "$NS_A"
        kubectl get serviceaccount -n "$NS_B"
        exit 1
    fi
    sleep $INTERVAL
done
echo "Resources of $NS_B and of mr-namespace-unmatch-other were kept"

kubectl delete managedresource mr-namespace-unmatch mr-namespace-unmatch-other
kubectl delete namespace $NS_A $NS_B
