#!/usr/bin/env bash
set -uo pipefail

compose=(docker compose -f docker-compose.yml)
attempts="${SMOKE_ATTEMPTS:-30}"
delay="${SMOKE_DELAY_SECONDS:-2}"
tmp_dir="$(mktemp -d)"
passed=0
failed=0
skipped=0
declare -a failures=()

cleanup() {
    rm -rf "$tmp_dir"
}
trap cleanup EXIT

mapfile -t configured_services < <("${compose[@]}" config --services 2>/dev/null)

has_service() {
    local wanted="$1"
    local service
    for service in "${configured_services[@]}"; do
        if [[ "$service" == "$wanted" ]]; then
            return 0
        fi
    done
    return 1
}

run_check() {
    local label="$1"
    local service="$2"
    shift 2

    if ! has_service "$service"; then
        printf 'SKIP  %s (%s is disabled)\n' "$label" "$service"
        skipped=$((skipped + 1))
        return 0
    fi

    local output="$tmp_dir/check-$passed-$failed-$skipped.log"
    local attempt
    for ((attempt = 1; attempt <= attempts; attempt++)); do
        if "$@" >"$output" 2>&1; then
            printf 'PASS  %s\n' "$label"
            passed=$((passed + 1))
            return 0
        fi
        sleep "$delay"
    done

    printf 'FAIL  %s\n' "$label"
    if [[ -s "$output" ]]; then
        tail -n 12 "$output" | sed 's/^/      /'
    fi
    failures+=("$label")
    failed=$((failed + 1))
    return 1
}

http_get() {
    curl --connect-timeout 2 --max-time 5 -fsS "$1"
}

check_nifi() {
    local username password token
    username="$("${compose[@]}" exec -T nifi printenv SINGLE_USER_CREDENTIALS_USERNAME)" || return 1
    password="$("${compose[@]}" exec -T nifi printenv SINGLE_USER_CREDENTIALS_PASSWORD)" || return 1
    token="$(curl --connect-timeout 2 --max-time 5 -kfsS \
        --data-urlencode "username=$username" \
        --data-urlencode "password=$password" \
        https://localhost:8082/nifi-api/access/token)" || return 1
    [[ -n "$token" ]] || return 1
    curl --connect-timeout 2 --max-time 5 -kfsS \
        -H "Authorization: Bearer $token" \
        https://localhost:8082/nifi-api/flow/about
}

check_kafka_flink_pipeline() {
    local topic="stilt-smoke-$(date +%s)-$$"
    local message="stilt smoke $(date +%s)-$$"
    local received flink_output

    "${compose[@]}" exec -T kafka /opt/kafka/bin/kafka-topics.sh \
        --bootstrap-server kafka:9092 --create --if-not-exists \
        --topic "$topic" --partitions 1 --replication-factor 1 >/dev/null || return 1

    printf '%s\n' "$message" | "${compose[@]}" exec -T kafka \
        /opt/kafka/bin/kafka-console-producer.sh \
        --bootstrap-server kafka:9092 --topic "$topic" || return 1

    received="$("${compose[@]}" exec -T kafka \
        /opt/kafka/bin/kafka-console-consumer.sh \
        --bootstrap-server kafka:9092 --topic "$topic" --from-beginning \
        --max-messages 1 --timeout-ms 10000 2>/dev/null)" || return 1
    [[ "$received" == "$message" ]] || return 1

    flink_output="$(
        printf '%s\n' \
            "SET 'execution.runtime-mode' = 'batch';" \
            "SET 'sql-client.execution.result-mode' = 'tableau';" \
            "CREATE TABLE kafka_source (message STRING) WITH ('connector' = 'kafka', 'topic' = '$topic', 'properties.bootstrap.servers' = 'kafka:9092', 'properties.group.id' = '$topic', 'scan.startup.mode' = 'earliest-offset', 'scan.bounded.mode' = 'latest-offset', 'format' = 'raw');" \
            "SELECT message FROM kafka_source;" \
        | "${compose[@]}" exec -T flink-jobmanager \
            /opt/flink/bin/sql-client.sh -f /dev/stdin 2>&1
    )" || return 1

    "${compose[@]}" exec -T kafka /opt/kafka/bin/kafka-topics.sh \
        --bootstrap-server kafka:9092 --delete --topic "$topic" >/dev/null 2>&1 || true
    grep -Fq "$message" <<<"$flink_output"
}

if ((${#configured_services[@]} == 0)); then
    printf 'No generated services found. Run ./run.sh first.\n' >&2
    exit 1
fi

printf 'Stilt smoke tests (%d generated services)\n\n' "${#configured_services[@]}"

run_check "Airflow health API" airflow \
    http_get http://localhost:8080/health

run_check "PostgreSQL query" postgres \
    "${compose[@]}" exec -T postgres sh -ec \
    'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" && psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc "SELECT 1" | grep -qx 1'

run_check "Redis PING" redis \
    "${compose[@]}" exec -T redis redis-cli ping

run_check "ClickHouse query" clickhouse \
    "${compose[@]}" exec -T clickhouse clickhouse-client --query "SELECT 1"

run_check "MinIO live health" minio \
    http_get http://localhost:9000/minio/health/live

run_check "ZooKeeper server status" zookeeper \
    "${compose[@]}" exec -T zookeeper zkServer.sh status

run_check "Kafka broker metadata" kafka \
    "${compose[@]}" exec -T kafka /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server kafka:9092 --list

run_check "NiFi authenticated HTTPS API" nifi check_nifi

run_check "Spark master UI" spark-master \
    http_get http://localhost:8083/

run_check "Spark worker registered" spark-worker \
    bash -ec 'curl --connect-timeout 2 --max-time 5 -fsS http://localhost:8083/json/ | grep -Eq '\''"aliveworkers"[[:space:]]*:[[:space:]]*[1-9]'\'''

run_check "Flink JobManager overview" flink-jobmanager \
    http_get http://localhost:8084/overview

run_check "Flink TaskManager registered" flink-taskmanager \
    bash -ec 'curl --connect-timeout 2 --max-time 5 -fsS http://localhost:8084/taskmanagers | grep -Eq '\''"taskmanagers"[[:space:]]*:[[:space:]]*\[[^]]+'\'''

run_check "Dagster GraphQL endpoint" dagster \
    curl --connect-timeout 2 --max-time 5 -fsS \
    -H 'Content-Type: application/json' \
    -d '{"query":"{ version }"}' http://localhost:3000/graphql

run_check "Trino server info" trino \
    http_get http://localhost:8086/v1/info

run_check "Elasticsearch cluster health" elasticsearch \
    http_get http://localhost:9200/_cluster/health

run_check "Kibana status API" kibana \
    http_get http://localhost:5601/api/status

run_check "Superset health" superset \
    http_get http://localhost:8088/health

run_check "Metabase health" metabase \
    http_get http://localhost:3001/api/health

run_check "Grafana health" grafana \
    http_get http://localhost:3002/api/health

run_check "Prometheus health" prometheus \
    http_get http://localhost:9090/-/healthy

if has_service kafka && has_service flink-jobmanager; then
    run_check "Kafka produce/consume and Flink SQL pipeline" kafka \
        check_kafka_flink_pipeline
else
    printf 'SKIP  Kafka produce/consume and Flink SQL pipeline (services disabled)\n'
    skipped=$((skipped + 1))
fi

printf '\nSummary: %d passed, %d failed, %d skipped\n' "$passed" "$failed" "$skipped"
if ((failed > 0)); then
    printf 'Failed checks:\n'
    printf '  - %s\n' "${failures[@]}"
    exit 1
fi
