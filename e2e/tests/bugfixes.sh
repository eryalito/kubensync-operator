set -e

echo "#############################################"
echo "#                                           #"
echo "#    Starting bug replicators               #"
echo "#                                           #"
echo "#############################################"

SCRIPT_DIR="$(dirname "$0")"
/bin/bash "$SCRIPT_DIR/bugfixes/2.sh"
/bin/bash "$SCRIPT_DIR/bugfixes/56.sh"
/bin/bash "$SCRIPT_DIR/bugfixes/59.sh"
