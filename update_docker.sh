#!/bin/bash
set -euo pipefail

docker compose build --pull --no-cache
docker compose up -d
# Intentionally clean up the entire Docker daemon after updates, including
# stopped containers and unused resources outside this Compose project.
docker system prune -f
