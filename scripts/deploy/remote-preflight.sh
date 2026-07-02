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

table_exists() {
    local db="$1"
    local table="$2"

    sqlite3 "$db" "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = '$table' LIMIT 1;" | grep -qx "1"
}

print_table_count() {
    local db="$1"
    local table="$2"

    if table_exists "$db" "$table"; then
        local count
        count="$(sqlite3 "$db" "SELECT COUNT(*) FROM \"$table\";")"
        echo "$table=$count"
    else
        echo "$table=(missing)"
    fi
}

print_db_summary() {
    local db="$1"

    echo ""
    echo "Database summary for: $db"

    echo ""
    echo "__EFMigrationsHistory:"
    if table_exists "$db" "__EFMigrationsHistory"; then
        sqlite3 "$db" "SELECT MigrationId FROM __EFMigrationsHistory ORDER BY MigrationId;"
    else
        echo "(missing)"
    fi

    echo ""
    echo "WorkScheduleEntries schema:"
    if table_exists "$db" "WorkScheduleEntries"; then
        sqlite3 "$db" ".schema WorkScheduleEntries"
    else
        echo "(missing)"
    fi

    echo ""
    echo "Key table counts:"
    print_table_count "$db" "Users"
    print_table_count "$db" "Reservations"
    print_table_count "$db" "Notifications"
    print_table_count "$db" "ChatworkDeliveryLogs"
    print_table_count "$db" "RoomConflictRecords"
    print_table_count "$db" "WorkScheduleEntries"

    echo ""
    echo "integrity_check:"
    integrity_check "$db"
}

run_bundle_against_copy() {
    local bundle="$1"
    local db="$2"

    chmod 700 "$bundle"
    "$bundle" --connection "Data Source=$db"
}

require_root
require_backup_dir
require_deploy_mode
require_command sqlite3
require_command python3

readonly ARTIFACTS_DIR="$BACKUP_DIR/artifacts"
readonly PREFLIGHT_OK="$BACKUP_DIR/preflight.ok"
readonly PREFLIGHT_DB_BACKUP="$BACKUP_DIR/app.db.preflight.bak"
readonly PREFLIGHT_MIGRATION_COPY="$BACKUP_DIR/app.db.preflight-migration-copy.db"
readonly BUNDLE_PATH="$ARTIFACTS_DIR/efbundle-linux-x64"

rm -f "$PREFLIGHT_OK"
umask 077

[ -d "$BACKUP_DIR" ] || fail "BACKUP_DIR does not exist: $BACKUP_DIR"
[ -d "$ARTIFACTS_DIR" ] || fail "Artifacts directory does not exist: $ARTIFACTS_DIR"
[ -d "$API_PATH" ] || fail "API path does not exist: $API_PATH"
[ -d "$WEB_ROOT" ] || fail "Web root does not exist: $WEB_ROOT"
[ -f "$DB_PATH" ] || fail "Production DB does not exist: $DB_PATH"
[ -f "$ARTIFACTS_DIR/api.zip" ] || fail "Missing artifact: $ARTIFACTS_DIR/api.zip"
[ -f "$ARTIFACTS_DIR/web.zip" ] || fail "Missing artifact: $ARTIFACTS_DIR/web.zip"

if [ "$DEPLOY_MODE" = "migration" ]; then
    [ -f "$BUNDLE_PATH" ] || fail "Missing migration bundle: $BUNDLE_PATH"
fi

echo "Preflight"
echo "Backup dir:  $BACKUP_DIR"
echo "Deploy mode: $DEPLOY_MODE"
echo "API service: $API_SERVICE_NAME"
echo "API path:    $API_PATH"
echo "DB path:     $DB_PATH"
echo "Web root:    $WEB_ROOT"

echo ""
echo "Creating production DB preflight backup..."
run_sqlite_backup "$DB_PATH" "$PREFLIGHT_DB_BACKUP"

print_db_summary "$PREFLIGHT_DB_BACKUP"

if [ "$DEPLOY_MODE" = "migration" ]; then
    echo ""
    echo "Testing migration bundle against copied DB. Production DB is not modified in preflight."
    [ ! -e "$PREFLIGHT_MIGRATION_COPY" ] || fail "Migration copy DB already exists: $PREFLIGHT_MIGRATION_COPY"
    cp -p "$PREFLIGHT_DB_BACKUP" "$PREFLIGHT_MIGRATION_COPY"
    run_bundle_against_copy "$BUNDLE_PATH" "$PREFLIGHT_MIGRATION_COPY"
    print_db_summary "$PREFLIGHT_MIGRATION_COPY"
fi

cat > "$PREFLIGHT_OK" <<EOF
preflight_ok=1
created_at_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
deploy_mode=$DEPLOY_MODE
backup_dir=$BACKUP_DIR
db_path=$DB_PATH
EOF

chmod 600 "$PREFLIGHT_OK"

echo ""
echo "Preflight completed successfully."
echo "Created: $PREFLIGHT_OK"
