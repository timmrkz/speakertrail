#!/bin/bash
# Deploys one image to Scaleway: the web interface as a Serverless Container
# and the nightly crawl as a Serverless Job. Creates what is missing, updates
# what exists, so it is safe to run on every merge.
#
# Needs the scw CLI configured (SCW_ACCESS_KEY, SCW_SECRET_KEY,
# SCW_DEFAULT_PROJECT_ID, SCW_DEFAULT_ORGANIZATION_ID), jq, and:
#   IMAGE             the pushed image, e.g. rg.fr-par.scw.cloud/speakertrail/speakertrail:<sha>
#   DATABASE_URL      the Serverless SQL Database connection string
#   UI_PASSWORD_HASH  from `speakertrail hash-password`
#   SESSION_SECRET    a long random string
set -euo pipefail

REGION=${SCW_REGION:-fr-par}
NAMESPACE=${SCW_NAMESPACE:-speakertrail}
CONTAINER=speakertrail-web
JOB=speakertrail-nightly
SCHEDULE=${NIGHTLY_SCHEDULE:-"0 5 * * *"}

: "${IMAGE:?IMAGE is required}"
: "${DATABASE_URL:?DATABASE_URL is required}"
: "${UI_PASSWORD_HASH:?UI_PASSWORD_HASH is required}"
: "${SESSION_SECRET:?SESSION_SECRET is required}"

log() { echo "[deploy] $*"; }

# Container namespace.
ns_id=$(scw container namespace list name="$NAMESPACE" region="$REGION" -o json | jq -r '.[0].id // empty')
if [ -z "$ns_id" ]; then
	log "creating container namespace $NAMESPACE"
	ns_id=$(scw container namespace create name="$NAMESPACE" region="$REGION" -o json | jq -r '.id')
	# A new namespace needs a moment before containers can be added.
	for _ in $(seq 1 30); do
		status=$(scw container namespace get "$ns_id" region="$REGION" -o json | jq -r '.status')
		[ "$status" = "ready" ] && break
		sleep 5
	done
fi

# Web interface.
secrets=(
	secret-environment-variables.0.key=DATABASE_URL "secret-environment-variables.0.value=$DATABASE_URL"
	secret-environment-variables.1.key=UI_PASSWORD_HASH "secret-environment-variables.1.value=$UI_PASSWORD_HASH"
	secret-environment-variables.2.key=SESSION_SECRET "secret-environment-variables.2.value=$SESSION_SECRET"
)
c_id=$(scw container container list namespace-id="$ns_id" name="$CONTAINER" region="$REGION" -o json | jq -r --arg n "$CONTAINER" '[.[] | select(.name == $n)][0].id // empty')
if [ -z "$c_id" ]; then
	log "creating container $CONTAINER"
	c_id=$(scw container container create namespace-id="$ns_id" name="$CONTAINER" region="$REGION" \
		registry-image="$IMAGE" port=8080 min-scale=0 max-scale=1 memory-limit=1024 cpu-limit=1000 \
		privacy=public http-option=redirected \
		"${secrets[@]}" -o json | jq -r '.id')
else
	log "updating container $CONTAINER"
	scw container container update "$c_id" region="$REGION" registry-image="$IMAGE" "${secrets[@]}" -o json >/dev/null
fi
domain=$(scw container container get "$c_id" region="$REGION" -o json | jq -r '.domain_name')

# Nightly crawl. Serverless Jobs take secrets from Secret Manager only, so
# the database address is passed as a plain variable of this private job.
job_id=$(scw jobs definition list region="$REGION" -o json | jq -r --arg n "$JOB" '[.[] | select(.name == $n)][0].id // empty')
job_args=(
	image-uri="$IMAGE" cpu-limit=1000 memory-limit=2048
	startup-command.0=/app/speakertrail args.0=nightly
	cron-schedule.schedule="$SCHEDULE" cron-schedule.timezone=Europe/Berlin
	"environment-variables.DATABASE_URL=$DATABASE_URL"
)
if [ -z "$job_id" ]; then
	log "creating job $JOB"
	scw jobs definition create name="$JOB" region="$REGION" "${job_args[@]}" -o json >/dev/null
else
	log "updating job $JOB"
	scw jobs definition update "$job_id" region="$REGION" "${job_args[@]}" -o json >/dev/null
fi

log "done: https://$domain"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	echo "Deployed \`$IMAGE\` to https://$domain" >>"$GITHUB_STEP_SUMMARY"
fi
