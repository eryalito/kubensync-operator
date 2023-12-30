set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests cleanup...          #"
echo "#                                           #"
echo "#############################################"

kubectl delete -k deploy
kubectl delete -f render/rbac.yaml
kubectl delete namespace test-kubensync