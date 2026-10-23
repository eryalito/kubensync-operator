#!/bin/bash

set -e

echo "#############################################"
echo "#                                           #"
echo "#        Running Data Watch Tests           #"
echo "#                                           #"
echo "#############################################"

NAMESPACE="test-kubensync-datawatch"
SRC_NAMESPACE="test-kubensync-datawatch-src"
MANAGER_NAMESPACE="kubensync-system"
MAX_WAIT=30
# Longer than the debounce plus a reconcile, so a change that should not be picked up has had time to be.
QUIET_WAIT=10
INTERVAL=1

MANAGER_POD=$(kubectl get pods -n "$MANAGER_NAMESPACE" -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}')
RESTARTS_BEFORE=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.containerStatuses[0].restartCount}')

# create_mr NAME TARGET WATCH...: renders ConfigMap TARGET from the source, with one data entry per WATCH value.
create_mr() {
    local name=$1 target=$2
    shift 2
    local data=""
    local i=0
    for watch in "$@"; do
        data+="
      - name: src$i
        type: ConfigMap
        ref:
          name: src
          namespace: $SRC_NAMESPACE
        watch: $watch"
        i=$((i + 1))
    done
    kubectl create -f - <<EOF
apiVersion: automation.kubensync.com/v1alpha1
kind: ManagedResource
metadata:
  name: $name
spec:
  namespaceSelector:
    regex: "^$NAMESPACE\$"
  template:
    data:$data
    literal: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: $target
        namespace: {{ .Namespace.Name }}
      data:
        value: {{ index .Data.src0 "value" | quote }}
EOF
}

set_source() {
    kubectl patch configmap src -n $SRC_NAMESPACE --type merge -p "{\"data\":{\"value\":\"$1\"}}" > /dev/null
}

target_value() {
    kubectl get configmap "$1" -n $NAMESPACE -o jsonpath='{.data.value}' 2>/dev/null || true
}

wait_for_value() {
    for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
        if [ "$(target_value "$1")" == "$2" ]; then
            echo "$1 has value $2"
            return 0
        fi
        sleep $INTERVAL
    done
    echo "Error: $1 has value '$(target_value "$1")', expected $2"
    exit 1
}

expect_value_kept() {
    for (( i=0; i<$QUIET_WAIT; i+=INTERVAL )); do
        if [ "$(target_value "$1")" != "$2" ]; then
            echo "Error: $1 changed to '$(target_value "$1")' although its data is not watched"
            exit 1
        fi
        sleep $INTERVAL
    done
    echo "$1 kept value $2"
}

# set_watch MR INDEX true|false, then waits for the reconcile of that spec, so it cannot be mistaken for a watch.
set_watch() {
    kubectl patch managedresource "$1" --type json -p "[{\"op\":\"add\",\"path\":\"/spec/template/data/$2/watch\",\"value\":$3}]" > /dev/null
    local generation
    generation=$(kubectl get managedresource "$1" -o jsonpath='{.metadata.generation}')
    for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
        observed=$(kubectl get managedresource "$1" -o jsonpath='{.status.conditions[?(@.type=="Ready")].observedGeneration}')
        if [ "$observed" == "$generation" ]; then
            echo "$1 data $2 watch set to $3"
            return 0
        fi
        sleep $INTERVAL
    done
    echo "Error: $1 was not reconciled after setting data $2 watch to $3"
    exit 1
}

kubectl create namespace $NAMESPACE
kubectl create namespace $SRC_NAMESPACE
kubectl create configmap src -n $SRC_NAMESPACE --from-literal=value=1

echo "One ManagedResource referencing the same ConfigMap twice, both watched"
create_mr mr-data-watch-1 target-1 true true
wait_for_value target-1 1
set_source 2
wait_for_value target-1 2

echo "One reference stops watching, the other one keeps the watch"
set_watch mr-data-watch-1 0 false
set_source 3
wait_for_value target-1 3

echo "Both references stop watching"
set_watch mr-data-watch-1 1 false
set_source 4
expect_value_kept target-1 3

echo "One reference watches again"
set_watch mr-data-watch-1 0 true
set_source 5
wait_for_value target-1 5

echo "A second ManagedResource watches the same ConfigMap"
create_mr mr-data-watch-2 target-2 true
wait_for_value target-2 5
set_source 6
wait_for_value target-1 6
wait_for_value target-2 6

echo "Deleting the first ManagedResource keeps the watch of the second one"
kubectl delete managedresource mr-data-watch-1
set_source 7
wait_for_value target-2 7

echo "Deleting the last ManagedResource"
kubectl delete managedresource mr-data-watch-2
set_source 8
sleep $QUIET_WAIT

phase=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
RESTARTS_AFTER=$(kubectl get pod "$MANAGER_POD" -n "$MANAGER_NAMESPACE" -o jsonpath='{.status.containerStatuses[0].restartCount}')
if [ "$phase" != "Running" ] || [ "$RESTARTS_AFTER" != "$RESTARTS_BEFORE" ]; then
    echo "Error: manager pod $MANAGER_POD is $phase with $RESTARTS_AFTER restarts, had $RESTARTS_BEFORE"
    exit 1
fi
echo "Manager pod $MANAGER_POD is running with $RESTARTS_AFTER restarts"

kubectl delete namespace $NAMESPACE $SRC_NAMESPACE
