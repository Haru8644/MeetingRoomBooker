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
        fail "Run this script as root, for example with: sudo env BACKUP_DIR=... DEPLOY_MODE=... bash $0"
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

require_deploy_mode() {
    if [ -z "${DEPLOY_MODE:-}" ]; then
        fail "DEPLOY_MODE is required. Use api-web or migration."
    fi

    case "$DEPLOY_MODE" in
        api-web|migration) ;;
        *) fail "DEPLOY_MODE must be api-web or migration: $DEPLOY_MODE" ;;
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

run_sqlite_backup() {
    local source_db="$1"
    local backup_db="$2"

    [ -f "$source_db" ] || fail "SQLite source DB does not exist: $source_db"
    [ ! -e "$backup_db" ] || fail "Backup DB already exists: $backup_db"

    sqlite3 "$source_db" ".backup '$backup_db'"
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

extract_zip_normalized() {
    local zip_path="$1"
    local destination="$2"

    [ -f "$zip_path" ] || fail "Zip file does not exist: $zip_path"

    rm -rf "$destination"
    mkdir -p "$destination"

    python3 - "$zip_path" "$destination" <<'PY'
import os
import shutil
import sys
import zipfile

zip_path = sys.argv[1]
destination = sys.argv[2]
destination_real = os.path.realpath(destination)

with zipfile.ZipFile(zip_path) as archive:
    for member in archive.infolist():
        normalized = member.filename.replace("\\", "/")
        parts = [part for part in normalized.split("/") if part not in ("", ".")]

        if not parts:
            continue

        if any(part == ".." for part in parts):
            raise SystemExit(f"Unsafe zip entry: {member.filename}")

        target = os.path.realpath(os.path.join(destination, *parts))
        if target != destination_real and not target.startswith(destination_real + os.sep):
            raise SystemExit(f"Unsafe zip target: {member.filename}")

        if member.is_dir() or normalized.endswith("/"):
            os.makedirs(target, exist_ok=True)
            continue

        os.makedirs(os.path.dirname(target), exist_ok=True)
        with archive.open(member) as source, open(target, "wb") as output:
            shutil.copyfileobj(source, output)
PY
}

assert_no_db_files_in_api_artifact() {
    local directory="$1"
    local found

    found="$(find "$directory" -type f \( -name "*.db" -o -name "*.db-shm" -o -name "*.db-wal" \) -print -quit)"
    if [ -n "$found" ]; then
        fail "API artifact contains a SQLite database file and will not be deployed: $found"
    fi
}

copy_required_runtime_files_to_new_api() {
    local new_api="$1"

    [ -f "$DB_PATH" ] || fail "Production DB is missing before API swap: $DB_PATH"
    [ -f "$API_PATH/appsettings.json" ] || fail "Production appsettings.json is missing: $API_PATH/appsettings.json"

    cp -p "$DB_PATH" "$new_api/app.db"
    cp -p "$API_PATH/appsettings.json" "$new_api/appsettings.json"
}

copy_optional_runtime_files_to_new_web() {
    local new_web="$1"
    local production_web_settings="$WEB_ROOT/appsettings.json"

    if [ -f "$production_web_settings" ]; then
        echo "Preserving production Web appsettings.json: $production_web_settings"
        cp -p "$production_web_settings" "$new_web/appsettings.json"
    else
        echo "Production Web appsettings.json was not found; skipping Web settings preservation: $production_web_settings"
    fi
}

run_migration_bundle() {
    local bundle="$1"

    chmod 700 "$bundle"
    "$bundle" --connection "Data Source=$DB_PATH"
}

print_final_status() {
    echo ""
    echo "systemctl status:"
    systemctl --no-pager status "$API_SERVICE_NAME" || true

    echo ""
    echo "Listening sockets:"
    ss -lntp || true

    echo ""
    echo "curl web root:"
    curl -fsSIL --max-time 10 http://127.0.0.1/ || true

    echo ""
    echo "Recent API journal:"
    journalctl -u "$API_SERVICE_NAME" -n 80 --no-pager || true

    echo ""
    echo "Final DB integrity_check:"
    integrity_check "$DB_PATH"
}

require_root
require_backup_dir
require_deploy_mode
require_command sqlite3
require_command python3
require_command systemctl
require_command ss
require_command curl
require_command journalctl

readonly ARTIFACTS_DIR="$BACKUP_DIR/artifacts"
readonly SCRIPTS_DIR="$BACKUP_DIR/scripts"
readonly PREFLIGHT_OK="$BACKUP_DIR/preflight.ok"
readonly DEPLOY_FIXED="$BACKUP_DIR/deploy-fixed"
readonly NEW_API_DIR="$DEPLOY_FIXED/api"
readonly NEW_WEB_DIR="$DEPLOY_FIXED/web"
readonly FINAL_DB_BACKUP="$BACKUP_DIR/app.db.final-before-production-migration.bak"
readonly API_PRE_DEPLOY="$BACKUP_DIR/api.pre-deploy"
readonly WEB_PRE_DEPLOY="$BACKUP_DIR/web.pre-deploy"
readonly API_RUNTIME_BEFORE_SWAP="$BACKUP_DIR/api.runtime-before-swap"
readonly WEB_RUNTIME_BEFORE_SWAP="$BACKUP_DIR/web.runtime-before-swap"
readonly BUNDLE_PATH="$ARTIFACTS_DIR/efbundle-linux-x64"

[ -f "$PREFLIGHT_OK" ] || fail "preflight.ok is missing. Run remote-preflight.sh successfully before deploy-prod.sh."
[ -d "$ARTIFACTS_DIR" ] || fail "Artifacts directory does not exist: $ARTIFACTS_DIR"
[ -d "$SCRIPTS_DIR" ] || fail "Scripts directory does not exist: $SCRIPTS_DIR"
[ -d "$API_PATH" ] || fail "API path does not exist: $API_PATH"
[ -d "$WEB_ROOT" ] || fail "Web root does not exist: $WEB_ROOT"
[ -f "$DB_PATH" ] || fail "Production DB does not exist: $DB_PATH"
[ -f "$ARTIFACTS_DIR/api.zip" ] || fail "Missing artifact: $ARTIFACTS_DIR/api.zip"
[ -f "$ARTIFACTS_DIR/web.zip" ] || fail "Missing artifact: $ARTIFACTS_DIR/web.zip"

if [ "$DEPLOY_MODE" = "migration" ]; then
    [ -f "$BUNDLE_PATH" ] || fail "Missing migration bundle: $BUNDLE_PATH"
fi

echo "Production deploy"
echo "Backup dir:  $BACKUP_DIR"
echo "Deploy mode: $DEPLOY_MODE"
echo "API service: $API_SERVICE_NAME"
echo "API path:    $API_PATH"
echo "DB path:     $DB_PATH"
echo "Web root:    $WEB_ROOT"

echo ""
echo "Extracting artifacts with normalized zip paths..."
extract_zip_normalized "$ARTIFACTS_DIR/api.zip" "$NEW_API_DIR"
extract_zip_normalized "$ARTIFACTS_DIR/web.zip" "$NEW_WEB_DIR"

[ -f "$NEW_API_DIR/MeetingRoomBooker.Api.dll" ] || fail "API artifact is missing MeetingRoomBooker.Api.dll."
[ -f "$NEW_WEB_DIR/index.html" ] || fail "Web artifact is missing index.html."
[ -d "$NEW_WEB_DIR/_framework" ] || fail "Web artifact is missing _framework."
assert_no_db_files_in_api_artifact "$NEW_API_DIR"

confirm_exact "STOP" "The API service will be stopped before the final database backup."
systemctl stop "$API_SERVICE_NAME"

echo ""
echo "Creating final DB backup after API stop..."
run_sqlite_backup "$DB_PATH" "$FINAL_DB_BACKUP"
echo "Final backup integrity_check:"
integrity_check "$FINAL_DB_BACKUP"

[ ! -e "$API_PRE_DEPLOY" ] || fail "Pre-deploy API backup already exists: $API_PRE_DEPLOY"
[ ! -e "$WEB_PRE_DEPLOY" ] || fail "Pre-deploy Web backup already exists: $WEB_PRE_DEPLOY"

echo ""
echo "Backing up current API and Web directories..."
cp -a "$API_PATH" "$API_PRE_DEPLOY"
cp -a "$WEB_ROOT" "$WEB_PRE_DEPLOY"

if [ "$DEPLOY_MODE" = "migration" ]; then
    confirm_exact "APPLY" "The migration bundle will now be applied to the production DB."
    run_migration_bundle "$BUNDLE_PATH"

    echo ""
    echo "Production DB integrity_check after migration:"
    integrity_check "$DB_PATH"
else
    echo ""
    echo "DEPLOY_MODE=api-web, so production migration is skipped."
fi

echo ""
echo "Copying production app.db and appsettings.json into the new API publish directory..."
copy_required_runtime_files_to_new_api "$NEW_API_DIR"

echo ""
echo "Checking whether production Web appsettings.json should be preserved..."
copy_optional_runtime_files_to_new_web "$NEW_WEB_DIR"

confirm_exact "DEPLOY" "API and Web directories will now be swapped."

[ ! -e "$API_RUNTIME_BEFORE_SWAP" ] || fail "Runtime API swap backup already exists: $API_RUNTIME_BEFORE_SWAP"
[ ! -e "$WEB_RUNTIME_BEFORE_SWAP" ] || fail "Runtime Web swap backup already exists: $WEB_RUNTIME_BEFORE_SWAP"

mv "$API_PATH" "$API_RUNTIME_BEFORE_SWAP"
mv "$NEW_API_DIR" "$API_PATH"
mv "$WEB_ROOT" "$WEB_RUNTIME_BEFORE_SWAP"
mv "$NEW_WEB_DIR" "$WEB_ROOT"

echo ""
echo "Starting API service..."
systemctl start "$API_SERVICE_NAME"
sleep 3

if ! systemctl is-active --quiet "$API_SERVICE_NAME"; then
    print_final_status
    fail "API service did not become active after deployment."
fi

print_final_status

cat > "$BACKUP_DIR/deploy.ok" <<EOF
deploy_ok=1
created_at_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
deploy_mode=$DEPLOY_MODE
backup_dir=$BACKUP_DIR
db_path=$DB_PATH
EOF

chmod 600 "$BACKUP_DIR/deploy.ok"

echo ""
echo "Deployment completed successfully."
