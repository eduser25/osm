#!/bin/bash

set -aueo pipefail

# shellcheck disable=SC1091
source .env

./demo/deploy-traffic-specs.sh
./demo/deploy-traffic-target.sh
./demo/deploy-traffic-split.sh


echo -e "Enable SMI Spec policies"
kubectl patch configmap -n "$K8S_NAMESPACE" osm-config -p '{"data": {"permissive_traffic_policy_mode": "false"}}'
