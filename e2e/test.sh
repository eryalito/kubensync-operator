set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting e2e tests...                  #"
echo "#                                           #"
echo "#############################################"

SCRIPT_DIR="$(dirname "$0")"
/bin/bash "$SCRIPT_DIR/tests/namespaced.sh"
/bin/bash "$SCRIPT_DIR/tests/labelselector.sh"
/bin/bash "$SCRIPT_DIR/tests/clusterscoped.sh"
/bin/bash "$SCRIPT_DIR/tests/cm-secret.sh"
/bin/bash "$SCRIPT_DIR/tests/data-resource.sh"
/bin/bash "$SCRIPT_DIR/tests/conditions.sh"
/bin/bash "$SCRIPT_DIR/tests/bugfixes.sh"