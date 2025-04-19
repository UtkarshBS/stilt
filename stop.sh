#!/bin/bash
docker compose -f docker-compose.yml down "$@"
rm -f stilt docker-compose.yml
echo "🚫 Platform stopped"
