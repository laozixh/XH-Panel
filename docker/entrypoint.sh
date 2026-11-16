#!/usr/bin/env sh
set -eu

APP_DIR="${XH_APP_DIR:-/app}"
LEGACY_DIR="${XH_LEGACY_DATA_DIR:-/data}"
DEFAULT_CONF_DIR="${APP_DIR}/defaults/conf"

# is_mountpoint PATH
# Returns 0 when PATH is a mount point (bind mount / volume / tmpfs).
# /proc/self/mountinfo field 5 is the mount point.
MOUNTINFO="${XH_MOUNTINFO:-/proc/self/mountinfo}"
is_mountpoint() {
  [ -r "${MOUNTINFO}" ] || return 1
  awk -v p="$1" '$5 == p { found = 1 } END { exit(found ? 0 : 1) }' "${MOUNTINFO}" 2>/dev/null
}

# Resolve DATA_DIR (database, uploads, runtime) and CONF_DIR (conf.ini).
#
# Order of preference:
#   1. XH_DATA_DIR / ANGE_DATA_DIR (explicit, conf lives in <dir>/conf)
#   2. /data when it really is a mount point (docker-compose ./data:/data)
#   3. <mounted /app/conf>/.xh-data, so a deployment that only mounts
#      /app/conf still keeps its database, uploads and runtime on the host.
#      Without this fallback those files live in the container writable layer
#      and every image upgrade silently resets the panel to the demo data.
#   4. /data (ephemeral, loudly warned about below)
if [ -n "${XH_DATA_DIR:-}" ]; then
  DATA_DIR="${XH_DATA_DIR}"
elif [ -n "${ANGE_DATA_DIR:-}" ]; then
  DATA_DIR="${ANGE_DATA_DIR}"
elif is_mountpoint "${LEGACY_DIR}"; then
  DATA_DIR="${LEGACY_DIR}"
elif is_mountpoint "${APP_DIR}/conf"; then
  DATA_DIR="${APP_DIR}/conf/.xh-data"
else
  DATA_DIR="${LEGACY_DIR}"
fi

if [ "${DATA_DIR}" = "${APP_DIR}/conf/.xh-data" ]; then
  # conf is the mounted directory itself, data goes into a hidden subdirectory
  CONF_DIR="${APP_DIR}/conf"
else
  CONF_DIR="${DATA_DIR}/conf"
fi

mkdir -p "${CONF_DIR}" "${DATA_DIR}/database" "${DATA_DIR}/uploads" "${DATA_DIR}/runtime/temp"

# One time migration: data left behind in the old location (for example a
# container that used to keep everything in its writable layer) is carried
# over before the app starts, so restarting after an upgrade does not lose
# the panel content.
migrate_legacy_data() {
  [ "${DATA_DIR}" = "${LEGACY_DIR}" ] && return
  [ -d "${LEGACY_DIR}" ] || return

  if [ ! -f "${DATA_DIR}/database/database.db" ] && [ -f "${LEGACY_DIR}/database/database.db" ]; then
    echo "[xh-panel] migrating database from ${LEGACY_DIR} to ${DATA_DIR}"
    cp -a "${LEGACY_DIR}/database/." "${DATA_DIR}/database/" || true
  fi
  if [ -d "${LEGACY_DIR}/uploads" ]; then
    cp -a "${LEGACY_DIR}/uploads/." "${DATA_DIR}/uploads/" 2>/dev/null || true
  fi
}
migrate_legacy_data

# conf_is_empty
# True when the conf directory holds no real configuration. The hidden
# ".xh-data" directory that this script creates inside a mounted /app/conf is
# ignored, otherwise a fresh install would never seed conf.ini.
conf_is_empty() {
  [ -d "${CONF_DIR}" ] || return 0
  for entry in "${CONF_DIR}"/* "${CONF_DIR}"/.[!.]*; do
    [ -e "${entry}" ] || continue
    base="${entry##*/}"
    [ "${base}" = ".xh-data" ] && continue
    return 1
  done
  return 0
}

# If this is a fresh volume, initialize config/db from image defaults/seed
# 1) conf
if conf_is_empty; then
  if [ -d "${DEFAULT_CONF_DIR}" ]; then
    cp -a "${DEFAULT_CONF_DIR}/." "${CONF_DIR}/" || true
  fi
fi

# 2) database
if [ ! -f "${DATA_DIR}/database/database.db" ]; then
  if [ -f "${APP_DIR}/seed/database/database.db" ]; then
    echo "[xh-panel] no database found in ${DATA_DIR}, seeding the demo database"
    cp -a "${APP_DIR}/seed/database/database.db" "${DATA_DIR}/database/database.db" || true
  fi
fi

# 3) uploads (only if empty)
if [ -z "$(ls -A "${DATA_DIR}/uploads" 2>/dev/null || true)" ] || { [ "$(ls -A "${DATA_DIR}/uploads" | wc -l | tr -d ' ')" = "1" ] && [ -f "${DATA_DIR}/uploads/.gitkeep" ]; }; then
  if [ -d "${APP_DIR}/seed/uploads" ]; then
    cp -a "${APP_DIR}/seed/uploads/." "${DATA_DIR}/uploads/" || true
  fi
fi

# Link runtime paths expected by the app (relative ./conf ./database ./uploads ./runtime)
link_dir() {
  src="$1"; dst="$2"
  if [ "${src}" = "${dst}" ]; then
    return
  fi
  if [ -L "${src}" ]; then
    rm -f "${src}" 2>/dev/null || true
    ln -s "${dst}" "${src}" || echo "[xh-panel] warning: cannot link ${src}"
    return
  fi
  if [ -e "${src}" ]; then
    # Do not blindly delete a real directory: it may be a legacy user bind
    # mount. Carry its content over, then replace it with the symlink.
    cp -a "${src}/." "${dst}/" 2>/dev/null || true
    rm -rf "${src}" 2>/dev/null || true
    ln -s "${dst}" "${src}" 2>/dev/null || echo "[xh-panel] warning: cannot link ${src}"
    return
  fi
  ln -s "${dst}" "${src}" || echo "[xh-panel] warning: cannot link ${src}"
}

link_dir "${APP_DIR}/conf" "${CONF_DIR}"
link_dir "${APP_DIR}/database" "${DATA_DIR}/database"
link_dir "${APP_DIR}/uploads" "${DATA_DIR}/uploads"
link_dir "${APP_DIR}/runtime" "${DATA_DIR}/runtime"

if ! is_mountpoint "${DATA_DIR}" && ! is_mountpoint "${CONF_DIR}"; then
  cat >&2 <<'WARN'
[xh-panel] WARNING: neither /data nor /app/conf is a mount point.
[xh-panel] The database, uploaded icons and logs live inside the container
[xh-panel] and will be LOST when the container is recreated.
[xh-panel] Mount a host directory, for example:
[xh-panel]   volumes:
[xh-panel]     - ./data:/data
WARN
fi

echo "[xh-panel] data directory: ${DATA_DIR}"
echo "[xh-panel] conf directory: ${CONF_DIR}"

exec "$@"
