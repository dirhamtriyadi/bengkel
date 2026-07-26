#!/bin/sh
set -eu

: "${API_IMAGE:?API_IMAGE wajib diisi}"
: "${WEB_IMAGE:?WEB_IMAGE wajib diisi}"

export API_IMAGE WEB_IMAGE
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d --remove-orphans
docker compose -f docker-compose.prod.yml ps
docker image prune -f --filter "until=168h"
