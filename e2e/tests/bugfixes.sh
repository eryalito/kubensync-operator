set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting bug replicators               #"
echo "#                                           #"
echo "#############################################"


NAMESPACE="test-kubensync"
MAX_WAIT=30
INTERVAL=1

echo "#56 - Split MRs correctly when having multiple hyphens in the resource"

# MR that creates a Certificate CM on the namespace
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
            kind: ConfigMap
            metadata:
                name: test-hyphen-certificate
                namespace: {{ .Namespace.Name }}
            data:
                certificate: |-
                    -----BEGIN CERTIFICATE-----
                    MIIDXzCCAkcCFGm2s7nWYvZPT3EeT44nNNdC86XIMA0GCSqGSIb3DQEBCwUAMGwx
                    CzAJBgNVBAYTAkVTMRIwEAYDVQQIDAlpbWFnaW5hcnkxITAfBgNVBAoMGEludGVy
                    bmV0IFdpZGdpdHMgUHR5IEx0ZDESMBAGA1UECwwJS3ViZU5TeW5jMRIwEAYDVQQD
                    DAlLdWJlTlN5bmMwHhcNMjUwNDIyMDg0OTA4WhcNMjYwNDIyMDg0OTA4WjBsMQsw
                    CQYDVQQGEwJFUzESMBAGA1UECAwJaW1hZ2luYXJ5MSEwHwYDVQQKDBhJbnRlcm5l
                    dCBXaWRnaXRzIFB0eSBMdGQxEjAQBgNVBAsMCUt1YmVOU3luYzESMBAGA1UEAwwJ
                    S3ViZU5TeW5jMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAsWQF9rf7
                    FwlzchLzK73YOrVMx/aYN8BuMJd8nnNJY9BuMMzwAdn18Ohs88usJ4PoQWbyygsx
                    M8PPcPZdqlraErFm6cLoYCLHSFzM9wmCFagY4LpwX9IAbFOmeQngG6BedfPjs4N4
                    uDzPu1DsJ3AJMzSMfQD8Qcp8PAtIthyar2nHF1v8Qa4/HHn9zWIBXnmwF6CGKSoz
                    Bo+tiypjoqg/RmD38IqyhHWFHmNBQeNfjWltxEFdK2sPI6GBSRtbTB0DGIRt4NTX
                    1yUYr9WB7uWzXeo4ku3T/fOBcxlPQRDIfAAJksCO33u02rKM1o40kp14/1a+8dcW
                    Z+3xfgk1/12hHwIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQBeNczIdruQjst4TzOR
                    ixZapGrmpLHlxJPsbwfyVb4Zk46UbecN9wkQaXhzJ93Dpq34XsGhlAPRv1+owzAM
                    2ohQkApKw3bTEQP24Eh+nPPMnC/xyPdAgjyrV3RBrXqFbM4s/JaFjrRphKLL+zJQ
                    SIeCan9YqeVHgwCp5dM52P6BZxWrdX6WSF7QJJbTs+fXkS4zdEIKzhh0rrxnFRtR
                    LunMnH2cTmqymqrY6gGSHv+rw1Q3o9ixhkf/9GuhA2Vm1rbFAP9dFKFEPJY5bzlP
                    RYpA6rC+FWDDJXELZp+5vPG04ik2SV6HSk3F/98BRdO9+cLnckfc3Siqf0P/DRlA
                    rs4l
                    -----END CERTIFICATE-----
EOF

valid=0
for (( i=0; i<$MAX_WAIT; i+=INTERVAL )); do
    if kubectl get configmap "test-hyphen-certificate" -n "$NAMESPACE" > /dev/null 2>&1; then
        echo "ConfigMap test-hyphen-certificate created"
        # Check if the ConfigMap has the correct data
        if kubectl get configmap test-hyphen-certificate -n "$NAMESPACE" -o jsonpath='{.data.certificate}' | grep -- "-----BEGIN CERTIFICATE-----"; then
            echo "ConfigMap test-hyphen-certificate has the correct data"
            valid=1
            break
        else
            echo "ConfigMap test-hyphen-certificate does not have the correct data"
            exit 1
        fi
    fi
    echo "Waiting for ConfigMap test-hyphen-certificate to be created..."
    sleep $INTERVAL
done
if [ $valid -eq 0 ]; then
    echo "ConfigMap test-hyphen-certificate was not created"
    exit 1
fi

kubectl delete managedresource managedresource-sample
