#!/bin/bash
set -euo pipefail

docker compose build --pull --no-cache
docker compose up -d
docker system prune -f
