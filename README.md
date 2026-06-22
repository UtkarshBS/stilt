# Stilt

**Stilt** is a lightweight local stack generator for Docker Compose. It reads a declarative configuration, generates `docker-compose.yml` and `.env`, and brings up the enabled services with a single command.

## 🚀 Features

- **Declarative** service definitions in `config/services.yaml`
- **Port mappings** centrally managed in `config/ports.yaml`
- **Enable/disable** any service via `plugins.conf`
- **Automatic** `.env` generation with secure random secrets
- **Single‑step** startup & teardown with `run.sh`
- **Bootstrap** prompt for Docker and Compose on supported systems
- **Inline custom-image builds** executed by the Go deployment tool
- **Built‑in** support for:
  - Airflow, Postgres, Redis  
  - ClickHouse, MinIO  
  - Zookeeper & Kafka  
  - NiFi  
  - Spark & Flink  
  - Dagster, Trino  
  - Elasticsearch, Kibana  
  - Superset, Metabase, Grafana  
  - Prometheus

## 📦 Requirements

- Go 1.20+  
- Docker CE & Docker Compose v2+  
- `run.sh` can install Docker and Compose on supported Linux, macOS, Windows, and WSL setups, preferring `yay`/`paru` on Arch-based systems

## 🛠 Installation

1. **Clone the repo**
   ```bash
   git clone git@github.com:UtkarshBS/stilt.git
   cd stilt
   ```

2. **Build the CLI**
   ```bash
   go build -o stilt ./cmd
   ```

3. **Configure services**
   - Edit `config/services.yaml` to customize images, versions, environment, and dependencies.
   - Edit `config/ports.yaml` to adjust host-to-container port mappings.
   - Toggle services in `plugins.conf` by setting `serviceName = enabled`.

## ▶️ Quick Start (Local)

Bring up the default stream-processing stack:
```bash
./run.sh
```

The default Flink image includes `flink-sql-connector-kafka:5.0.0-2.2`. Jobs running inside the stack connect to Kafka at `kafka:9092`; applications running on the host use `localhost:9092`.

Spark uses the Apache-maintained `apache/spark:3.5.8` image. Its Scala 2.12 build matches the Flink stack, and the standalone master is available to jobs at `spark://spark-master:7077`.

Run the smoke-test suite for every enabled service. The suite waits for readiness, reports all failures instead of stopping at the first one, and includes an end-to-end Kafka-to-Flink SQL check:
```bash
./smoke-test.sh
```

**Access your services**:
```bash
docker compose ps --format "table {{.Service}}\t{{.Ports}}"
```
Or point your browser to:
- Flink JobManager: http://localhost:8084

## ⏹ Teardown

```bash
./stop.sh
```

Remove all Stilt containers, volumes, images, generated files, and local data:

```bash
./stop.sh --purge
```

## ⏬ Project Layout

```
.
├── cmd/                # CLI entry point
├── config/             # service definitions & port mappings
│   ├── services.yaml
│   └── ports.yaml
├── plugins.conf        # enable/disable services
├── docker-compose.yml  # generated output
├── run.sh              # single-step runner
├── .env                # generated environment file
├── data/               # per-service data volumes
└── logs/               # logs directory
```

## 📄 License

This project is licensed under the Apache 2.0 License. See [LICENSE](LICENSE) for details.

## 🤝 Contributing

1. Fork the repo.  
2. Create a feature branch: `git checkout -b feature/foo`  
3. Commit changes: `git commit -am "Add feature foo"`  
4. Push branch: `git push origin feature/foo`  
5. Open a Pull Request.

We welcome contributions and issues—let’s build a vibrant data-platform community together!
