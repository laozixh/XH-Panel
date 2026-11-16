#!/usr/bin/env bash
# Recover XH-Panel / AnGe-Panel data from an old container whose /data was
# never mounted to the host.
#
#   ./recover-data.sh                        list candidate containers
#   ./recover-data.sh <container> [dest]     copy the data out
#
# Works on stopped containers too: it uses "docker cp", which reads the
# container's writable layer directly. The default dest is ./recovered-data.
set -euo pipefail

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need docker

list_candidates() {
  echo "Containers that look like a panel (running or stopped):"
  echo
  docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}' \
    | grep -Ei 'ange|xh-panel|sun-panel' || echo "  (none found)"
  echo
  echo "Also worth checking: docker ps -a   (the old container may be renamed)"
  echo "Re-run with a container name to copy its data out:"
  echo "  ./recover-data.sh <container> [dest]"
}

if [ $# -lt 1 ]; then
  list_candidates
  exit 0
fi

CONTAINER="$1"
DEST="${2:-./recovered-data}"

if ! docker inspect "$CONTAINER" >/dev/null 2>&1; then
  echo "Container not found: $CONTAINER" >&2
  echo
  list_candidates
  exit 1
fi

echo "Container : ${CONTAINER}"
echo "Image     : $(docker inspect -f '{{.Config.Image}}' "$CONTAINER")"
echo "Created   : $(docker inspect -f '{{.Created}}' "$CONTAINER")"
echo "Status    : $(docker inspect -f '{{.State.Status}}' "$CONTAINER")"
echo

# Where could the data be? New layout puts it in /data; the fallback layout
# puts it under a mounted /app/conf.
CANDIDATES="/data /app/database /app/conf/.xh-data"

FOUND=""
for p in $CANDIDATES; do
  if docker cp "${CONTAINER}:${p}/database/database.db" /dev/null >/dev/null 2>&1; then
    FOUND="$p"
    break
  fi
done

if [ -z "$FOUND" ]; then
  echo "No panel database found inside ${CONTAINER}." >&2
  echo "Checked: ${CANDIDATES}" >&2
  echo >&2
  echo "If the container was already recreated by the upgrade, its writable" >&2
  echo "layer is gone and the data can only come from a backup." >&2
  echo "Check for one of:" >&2
  echo "  - <your conf dir>/database/backup/database-*.db   (created from v1.0.3)" >&2
  echo "  - a host snapshot / NAS backup of the old container" >&2
  exit 1
fi

echo "Data found at ${FOUND} inside the container."
mkdir -p "$DEST"

echo "Copying ${CONTAINER}:${FOUND} -> ${DEST} ..."
docker cp "${CONTAINER}:${FOUND}/." "$DEST/"

echo
echo "Recovered:"
ls -la "$DEST"
echo
if [ -f "$DEST/database/database.db" ]; then
  echo "database.db: $(wc -c < "$DEST/database/database.db") bytes"
  echo "uploads    : $(find "$DEST/uploads" -type f 2>/dev/null | wc -l | tr -d ' ') files"
  echo
  echo "Put it back with:"
  echo "  docker compose -f /dockers/xh-panel/docker-compose.yml down"
  echo "  cp -a ${DEST}/database/database.db /dockers-date/xh-panel/conf/.xh-data/database/database.db"
  echo "  cp -a ${DEST}/uploads/. /dockers-date/xh-panel/conf/.xh-data/uploads/"
  echo "  docker compose -f /dockers/xh-panel/docker-compose.yml up -d"
  echo
  echo "Better: stop using that compose file and mount /data instead:"
  echo "  -v /dockers-date/xh-panel/data:/data"
else
  echo "database.db not found in the copied files, see the listing above." >&2
  exit 1
fi
