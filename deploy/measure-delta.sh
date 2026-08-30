#!/bin/bash
# Замер объёма дельты по сценариям. $1 — размер каталога.
N=${1:-500}
echo "=== каталог $N записей ==="
printf '%6s %8s %12s %12s\n' 'доля' 'записей' 'дельта' 'дельта+gzip'

for PCT in 0 1 5 50 100; do
  docker compose exec -T postgres psql -U parts_user -d parts -q -v n=$N < seed.sql > /dev/null
  CURSOR=$(docker compose exec -T postgres psql -U parts_user -d parts -tAc \
    "SELECT COALESCE(max(version), 0) FROM parts;")

  if [ "$PCT" -gt 0 ]; then
    CHANGED=$(docker compose exec -T postgres psql -U parts_user -d parts -tAc \
      "UPDATE parts SET material = material || ' (ред.)'
       WHERE id IN (SELECT id FROM parts WHERE deleted_at IS NULL
                    ORDER BY article LIMIT (SELECT count(*) * $PCT / 100
                    FROM parts WHERE deleted_at IS NULL));
       SELECT count(*) FROM parts WHERE version > $CURSOR;")
  else
    CHANGED=0
  fi

  RAW=$(curl -s -o /dev/null -w '%{size_download}' \
    "http://localhost:8080/api/v1/parts/delta?since_version=$CURSOR")
  GZ=$(curl -s -o /dev/null -H 'Accept-Encoding: gzip' -w '%{size_download}' \
    "http://localhost:8080/api/v1/gz/parts/delta?since_version=$CURSOR")

  printf '%5s%% %8s %12s %12s\n' "$PCT" "$CHANGED" "$RAW" "$GZ"
done
