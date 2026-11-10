#!/bin/sh
# Carries the people you kept in the Docker version of Speaker Trail over
# into the app. Once is enough, and again does no harm.
#
#   make carry-over
#
# The Docker version kept its database in a Docker volume,
# speakertrail_postgres. This reads the kept people out of it with
# Postgres 16 in a container, and hands them to the app, which adds them
# to its file as kept. It changes nothing in the old database. Docker is
# needed only for this, once. Afterwards Docker and the volume can go.
set -e
VOLUME=speakertrail_postgres
APPBIN=bin/speakertrail-app
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
	echo "Docker is not running. Start Docker Desktop once more for this, then run make carry-over again."
	exit 1
fi
if ! docker volume inspect "$VOLUME" >/dev/null 2>&1; then
	echo "There is no old database here, the volume $VOLUME is gone. Nothing to carry over."
	exit 0
fi
# The app holds its file in memory and would write over what this adds.
if pgrep -x speakertrail-app >/dev/null 2>&1; then
	echo "Quit Speaker Trail first, then run make carry-over again."
	exit 1
fi
[ -x "$APPBIN" ] || make --no-print-directory "$APPBIN" >/dev/null

# Everyone kept, with their LinkedIn profile and what it says.
QUERY="SELECT COALESCE(json_agg(json_build_object(
	'name', p.full_name, 'headline', p.headline, 'city', p.city, 'notes', p.notes,
	'kept_at', p.status_changed_at, 'first_seen', p.created_at,
	'linkedin', COALESCE(pr.url, ''), 'born_about', COALESCE(pr.born_about, 0), 'age_reason', COALESCE(pr.age_reason, ''),
	'roles', COALESCE(pr.current_roles, ''), 'last_active', pr.last_active_at) ORDER BY p.id), '[]')
FROM people p
LEFT JOIN LATERAL (SELECT url, born_about, age_reason, current_roles, last_active_at FROM profiles
	WHERE person_id = p.id AND platform = 'linkedin' AND review <> 'rejected'
	ORDER BY review = 'confirmed' DESC, id LIMIT 1) pr ON true
WHERE p.podcast_status NOT IN ('new', 'known', 'skipped')"

# The old app's database may still run, from make run in the Docker
# version. Then it is asked, because two databases on one volume would
# break it. Otherwise one is started on the volume for this alone.
running=$(docker ps -q --filter "volume=$VOLUME" | head -n 1)
if [ -n "$running" ]; then
	container=$running
	started=0
else
	container=speakertrail-carry-over
	docker rm -f "$container" >/dev/null 2>&1 || true
	echo "Opening the old database"
	docker run -d --rm --name "$container" -v "$VOLUME:/var/lib/postgresql/data" postgres:16 >/dev/null
	started=1
	trap 'docker stop "$container" >/dev/null 2>&1 || true' EXIT
	i=0
	until docker exec "$container" pg_isready -U speakertrail >/dev/null 2>&1; do
		i=$((i + 1))
		if [ "$i" -gt 60 ]; then
			echo "The old database did not start. docker logs $container says why."
			exit 1
		fi
		sleep 1
	done
fi

docker exec -i "$container" psql -U speakertrail -d speakertrail -At -v ON_ERROR_STOP=1 -c "$QUERY" | "$APPBIN" carry-over
if [ "$started" = 1 ]; then
	echo "The old database is closed again and unchanged."
fi
