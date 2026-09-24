#!/bin/bash
# Makes sure the Container Registry namespace exists and prints its endpoint,
# for example rg.fr-par.scw.cloud/speakertrail.
set -euo pipefail
REGION=${SCW_REGION:-fr-par}
NAMESPACE=${SCW_NAMESPACE:-speakertrail}

endpoint=$(scw registry namespace list name="$NAMESPACE" region="$REGION" -o json | jq -r --arg n "$NAMESPACE" '[.[] | select(.name == $n)][0].endpoint // empty')
if [ -z "$endpoint" ]; then
	endpoint=$(scw registry namespace create name="$NAMESPACE" is-public=false region="$REGION" -o json | jq -r '.endpoint')
fi
echo "$endpoint"
