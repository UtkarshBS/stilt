# Stilt

**Stilt** is a lightweight data-platform orchestrator that can spin up a full stack of services locally via Docker Compose. It reads a declarative configuration, generates `docker-compose.yml` & `.env`, and brings up everything with a single command.

## 🚀 Features

- **Declarative** service definitions in `config/services.yaml`
- **Port mappings** centrally managed in `config/ports.yaml`
- **Enable/disable** any service via `plugins.conf`
- **Automatic** `.env` generation with secure random secrets
- **Single‑step** startup & teardown with `run.sh` or the `stilt` CLI
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

## 🛠 Installation

1. **Clone the repo**
   ```bash
   git clone git@github.com:UtkarshBS/stilt.git
   cd stilt
   ```

2. **Build the CLI**
   ```bash
   go build -o stilt cmd/main.go
   ```

3. **Configure services**
   - Edit `config/services.yaml` to customize images, versions, environment, and dependencies.
   - Edit `config/ports.yaml` to adjust host-to-container port mappings.
   - Toggle services in `plugins.conf` by setting `serviceName = enabled`.

## ▶️ Quick Start (Local)

Bring up the full platform:
```bash
./run.sh
```

**Access your services**:
```bash
docker compose ps --format "table {{.Service}}\t{{.Ports}}"
```
Or point your browser to:
- Airflow:   http://localhost:8080
- MinIO:     http://localhost:9001
- ClickHouse: http://localhost:8123

## ⏹ Teardown

```bash
docker compose down -v
```

## ⏬ Project Layout

```
.
├── cmd/                # CLI entry point and subcommands
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