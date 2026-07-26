#!/bin/sh
set -eu

: "${API_IMAGE:?API_IMAGE wajib diisi}"
: "${WEB_IMAGE:?WEB_IMAGE wajib diisi}"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.prod.yml"
ENV_FILE_PATH=${ENV_FILE:-"$SCRIPT_DIR/.env.production"}
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "File environment tidak ditemukan: $ENV_FILE_PATH" >&2
  exit 1
fi

set -a
# File production merupakan pasangan KEY=VALUE yang kompatibel dengan shell.
. "$ENV_FILE_PATH"
set +a

case "${DATABASE_URL:-}" in
  *@postgres:5432/*) ;;
  *)
    echo "DATABASE_URL Docker wajib menggunakan host postgres dan port internal 5432." >&2
    echo "Contoh: postgres://USER:PASSWORD@postgres:5432/DB?sslmode=disable" >&2
    exit 1
    ;;
esac

if [ "${HTTP_PORT:-8080}" != "8080" ]; then
  echo "HTTP_PORT container wajib 8080. Atur port publik pada reverse proxy, bukan HTTP_PORT." >&2
  exit 1
fi

ENV_FILE=$ENV_FILE_PATH
export API_IMAGE WEB_IMAGE ENV_FILE
docker compose --env-file "$ENV_FILE_PATH" -f "$COMPOSE_FILE" pull
docker compose --env-file "$ENV_FILE_PATH" -f "$COMPOSE_FILE" up -d --remove-orphans
docker compose --env-file "$ENV_FILE_PATH" -f "$COMPOSE_FILE" ps
docker image prune -f --filter "until=168h"
