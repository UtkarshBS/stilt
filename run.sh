#!/bin/bash
set -e

# Load enabled services
plugins_file="plugins.conf"
services=$(awk -F= '/= enabled/ { gsub(/ /, "", $1); print $1 }' "$plugins_file")

# Initialize directories
mkdir -p logs
echo "Initializing data directories..."
for svc in $services; do
    mkdir -p "data/$svc"
done

# Set permissions
chmod -R 777 data

# Build binary
echo "🛠️  Building stilt..."
go build -o stilt cmd/main.go

# Generate configuration
echo "🚀 Generating configuration..."
./stilt generate

# Start services
echo "🐳 Starting containers..."
docker compose -f docker-compose.yml up -d --force-recreate

echo "✅ Platform running!"
echo "📦 Data stored in: ./data"
echo "🔑 Credentials in: .env"
echo "🌐 Access:"
docker compose ps --format "table {{.Service}}\t{{.Ports}}"
