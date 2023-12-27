set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests setup...            #"
echo "#                                           #"
echo "#############################################"

kubectl apply -k deploy
kubectl apply -f render/rbac.yaml
# wait for pods to get ready
kubectl rollout status -n kubensync-system deployment controller-manager

kubectl create namespace test-kubensync