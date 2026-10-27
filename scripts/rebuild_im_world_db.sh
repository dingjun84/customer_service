#!/usr/bin/env bash
# Rebuild demo DB for IM world (方案 A). Run from repo root on the Mac.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
TS="$(date +%Y%m%d_%H%M%S)"
BACKUP="data/backup/${TS}"
MAIN="data/ecommerce_support.db"
NEW="data/ecommerce_support.new.db"

mkdir -p "$BACKUP"
echo "== backup =="
for f in "$MAIN" "${MAIN}-wal" "${MAIN}-shm"; do
  if [[ -f "$f" ]]; then
    cp -p "$f" "$BACKUP/"
    echo "  copied $f"
  fi
done

LOCKED=0
if command -v lsof >/dev/null 2>&1; then
  if lsof "$MAIN" >/dev/null 2>&1; then
    LOCKED=1
    echo "main db is open by another process → seed into $NEW"
  fi
fi

TARGET="$MAIN"
if [[ "$LOCKED" -eq 1 ]]; then
  TARGET="$NEW"
  rm -f "$NEW" "${NEW}-wal" "${NEW}-shm"
fi

echo "== seed -reset → $TARGET =="
export APP_ENV="${APP_ENV:-dev}"
export DB_PATH="$TARGET"
go run ./cmd/seed -reset -artifacts testdata

echo "== restore JD articles + non-seed conversations from backup =="
BACKUP_DB="$BACKUP/$(basename "$MAIN")"
if [[ ! -f "$BACKUP_DB" ]]; then
  echo "no backup db, skip restore"
  exit 0
fi

RESTORE_SQL="$(mktemp)"
cat > "$RESTORE_SQL" <<SQL
ATTACH DATABASE '${BACKUP_DB}' AS bak;
INSERT OR IGNORE INTO knowledge_articles
  SELECT * FROM bak.knowledge_articles
  WHERE IFNULL(source,'') != 'synthetic' AND IFNULL(source,'') != '';
INSERT OR IGNORE INTO conversations
  SELECT * FROM bak.conversations c
  WHERE NOT EXISTS (SELECT 1 FROM conversations x WHERE x.id = c.id);
INSERT OR IGNORE INTO messages
  SELECT * FROM bak.messages m
  WHERE EXISTS (SELECT 1 FROM conversations c WHERE c.id = m.conversation_id)
    AND NOT EXISTS (SELECT 1 FROM messages x WHERE x.id = m.id);
INSERT OR IGNORE INTO tool_executions
  SELECT * FROM bak.tool_executions t
  WHERE EXISTS (SELECT 1 FROM conversations c WHERE c.id = t.conversation_id)
    AND NOT EXISTS (SELECT 1 FROM tool_executions x WHERE x.id = t.id);
INSERT OR IGNORE INTO service_tickets
  SELECT * FROM bak.service_tickets t
  WHERE NOT EXISTS (SELECT 1 FROM service_tickets x WHERE x.id = t.id);
INSERT OR IGNORE INTO pending_actions
  SELECT * FROM bak.pending_actions p
  WHERE NOT EXISTS (SELECT 1 FROM pending_actions x WHERE x.id = p.id);
INSERT OR IGNORE INTO message_annotations
  SELECT * FROM bak.message_annotations a
  WHERE NOT EXISTS (SELECT 1 FROM message_annotations x WHERE x.id = a.id);
INSERT OR IGNORE INTO action_audit_logs
  SELECT * FROM bak.action_audit_logs a
  WHERE NOT EXISTS (SELECT 1 FROM action_audit_logs x WHERE x.id = a.id);

SELECT 'restored_nonsynthetic_articles', COUNT(1) FROM knowledge_articles WHERE IFNULL(source,'') != 'synthetic' AND IFNULL(source,'') != '';
SELECT 'conversations_total', COUNT(1) FROM conversations;
SELECT 'messages_total', COUNT(1) FROM messages;
DETACH DATABASE bak;
SQL

sqlite3 "$TARGET" < "$RESTORE_SQL"
rm -f "$RESTORE_SQL"

echo "== done target=$TARGET backup=$BACKUP =="
if [[ "$LOCKED" -eq 1 ]]; then
  echo "NOTE: server still holds $MAIN — stop server then switch: mv $NEW $MAIN (and wal/shm)"
fi
