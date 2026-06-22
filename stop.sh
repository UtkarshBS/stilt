#!/usr/bin/env bash
set -euo pipefail

purge=false

if [ "${1:-}" = "--purge" ]; then
    purge=true
    shift
fi

if [ -f docker-compose.yml ]; then
    if [ "$purge" = true ]; then
        docker compose -f docker-compose.yml down --volumes --remove-orphans --rmi all
    else
        docker compose -f docker-compose.yml down --remove-orphans "$@"
    fi
elif [ "$purge" = false ]; then
    echo "No generated stack found."
fi

if [ "$purge" = true ]; then
    for container in $(docker ps -aq --filter label=com.docker.compose.project=stilt); do
        docker rm -f "$container"
    done

    for volume in $(docker volume ls -q --filter label=com.docker.compose.project=stilt); do
        docker volume rm "$volume"
    done

    for network in $(docker network ls -q --filter label=com.docker.compose.project=stilt); do
        docker network rm "$network"
    done

    if [ -f .stilt-images ]; then
        while IFS= read -r image; do
            if [ -n "$image" ]; then
                docker image rm "$image" >/dev/null 2>&1 || true
            fi
        done < .stilt-images
    fi

    rm -rf data logs java-test/.m2 java-test/producer/target java-test/flink-consumer/target
    rm -f .env .stilt-images stilt docker-compose.yml
    echo "Stilt data, containers, volumes, networks, and images removed."
else
    rm -f stilt docker-compose.yml
    echo "Stilt stopped."
fi
