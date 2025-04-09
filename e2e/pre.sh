set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests setup...            #"
echo "#                                           #"
echo "#############################################"

kubectl apply -f dist/install.yaml
kubectl apply -f dist/rbac.yaml
# wait for pods to get ready
kubectl rollout status -n kubensync-system deployment kubensync-controller-manager

kubectl create namespace test-kubensync