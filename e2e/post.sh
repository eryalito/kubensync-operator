set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests cleanup...          #"
echo "#                                           #"
echo "#############################################"

kubectl delete -f dist/install.yaml
kubectl delete -f dist/rbac.yaml
kubectl delete namespace test-kubensync