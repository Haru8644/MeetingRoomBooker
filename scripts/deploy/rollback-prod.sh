#!/usr/bin/env bash
set -Eeuo pipefail

readonly API_SERVICE_NAME="meetingroombooker-api.service"
readonly API_PATH="/opt/meetingroombooker/api"
readonly DB_PATH="/opt/meetingroombooker/api/app.db"
readonly WEB_ROOT="/var/www/meetingroombooker"
readonly BACKUP_ROOT="/opt/meetingroombooker/backups"

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "Required command is missing: $1"
}

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        fail "Run this script as root, for example with: sudo env BACKUP_DIR=... bash $0"
    fi
}

require_backup_dir() {
    if [ -z "${BACKUP_DIR:-}" ]; then
        fail "BACKUP_DIR is required."
    fi

    case "$BACKUP_DIR" in
        "$BACKUP_ROOT"/*) ;;
        *) fail "BACKUP_DIR must be under $BACKUP_ROOT: $BACKUP_DIR" ;;
    esac
}

confirm_exact() {
    local expected="$1"
    local message="$2"
    local actual

    echo ""
    echo "$message"
    printf "Type %s to continue: " "$expected"
    read -r actual

    if [ "$actual" != "$expected" ]; then
        fail "Confirmation did not match $expected."
    fi
}

integrity_check() {
    local db="$1"
    local result

    result="$(sqlite3 "$db" "PRAGMA integrity_check;")"
    echo "$result"

    if [ "$result" != "ok" ]; then
        fail "SQLite integrity_check failed for $db"
    fi
}

print_final_status() {
    echo ""
    echo "systemctl status:"
    systemctl --no-pager status "$API_SERVICE_NAME" || true

    echo ""
    echo "curl web root:"
    curl -fsSIL --max-time 10 http://127.0.0.1/ || true

    echo ""
    echo "Recent API journal:"
    journalctl -u "$API_SERVICE_NAME" -n 80 --no-pager || true

    echo ""
    echo "Restored DB integrity_check:"
    integrity_check "$DB_PATH"
}

require_root
require_backup_dir
require_command sqlite3
require_command systemctl
require_command curl
require_command journalctl

readonly FINAL_DB_BACKUP="$BACKUP_DIR/app.db.final-before-production-migration.bak"
readonly API_PRE_DEPLOY="$BACKUP_DIR/api.pre-deploy"
readonly WEB_PRE_DEPLOY="$BACKUP_DIR/web.pre-deploy"
readonly ROLLBACK_STASH="$BACKUP_DIR/rollback-current-$(date -u +%Y%m%d-%H%M%S)"

[ -f "$FINAL_DB_BACKUP" ] || fail "Final DB backup is missing: $FINAL_DB_BACKUP"
[ -d "$API_PRE_DEPLOY" ] || fail "Pre-deploy API backup is missing: $API_PRE_DEPLOY"
[ -d "$WEB_PRE_DEPLOY" ] || fail "Pre-deploy Web backup is missing: $WEB_PRE_DEPLOY"

echo "Production rollback"
echo "Backup dir: $BACKUP_DIR"
echo "API path:   $API_PATH"
echo "DB path:    $DB_PATH"
echo "Web root:   $WEB_ROOT"

confirm_exact "ROLLBACK" "This will stop the API and restore DB, API, and Web to the pre-deploy state."

echo ""
echo "Stopping API service..."
systemctl stop "$API_SERVICE_NAME"

mkdir -p "$ROLLBACK_STASH"

if [ -e "$API_PATH" ]; then
    mv "$API_PATH" "$ROLLBACK_STASH/api.current"
fi

if [ -e "$WEB_ROOT" ]; then
    mv "$WEB_ROOT" "$ROLLBACK_STASH/web.current"
fi

echo ""
echo "Restoring API directory..."
cp -a "$API_PRE_DEPLOY" "$API_PATH"

echo "Restoring DB backup to $DB_PATH..."
cp -p "$FINAL_DB_BACKUP" "$DB_PATH"
integrity_check "$DB_PATH"

echo "Restoring Web directory..."
cp -a "$WEB_PRE_DEPLOY" "$WEB_ROOT"

echo ""
echo "Starting API service..."
systemctl start "$API_SERVICE_NAME"
sleep 3

if ! systemctl is-active --quiet "$API_SERVICE_NAME"; then
    print_final_status
    fail "API service did not become active after rollback."
fi

print_final_status

cat > "$BACKUP_DIR/rollback.ok" <<EOF
rollback_ok=1
created_at_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
backup_dir=$BACKUP_DIR
db_path=$DB_PATH
stashed_current_runtime=$ROLLBACK_STASH
EOF

chmod 600 "$BACKUP_DIR/rollback.ok"

echo ""
echo "Rollback completed successfully."
echo "Current failed runtime, if any, was moved under: $ROLLBACK_STASH"
