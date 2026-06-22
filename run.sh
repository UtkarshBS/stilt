#!/usr/bin/env bash
set -e

prompt_yes_no() {
    read -r -p "$1 [y/N] " reply
    case "$reply" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) return 1 ;;
    esac
}

run_as_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    elif command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    else
        return 1
    fi
}

detect_platform() {
    case "$(uname -s)" in
        Linux)
            if grep -qi microsoft /proc/version 2>/dev/null || [ -n "${WSL_DISTRO_NAME:-}" ]; then
                echo wsl
            else
                echo linux
            fi
            ;;
        Darwin)
            echo macos
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo windows
            ;;
        *)
            echo unknown
            ;;
    esac
}

wait_for_docker() {
    i=0
    while [ "$i" -lt 60 ]; do
        if docker info >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
        i=$((i + 1))
    done
    return 1
}

docker_ready() {
    command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

compose_ready() {
    command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1
}

track_image() {
    image="$1"
    touch .stilt-images
    if ! grep -Fxq "$image" .stilt-images; then
        printf '%s\n' "$image" >> .stilt-images
    fi
}

reenter_docker_group() {
    if [ "${STILT_DOCKER_GROUP_REEXEC:-}" = "1" ] || ! command -v sg >/dev/null 2>&1; then
        echo "Docker group membership changed. Start a new login session and run ./run.sh again."
        exit 1
    fi

    script_path=$(printf '%q' "$0")
    working_directory=$(printf '%q' "$PWD")
    exec sg docker -c "cd $working_directory && STILT_DOCKER_GROUP_REEXEC=1 $script_path"
}

ensure_linux_docker_access() {
    user_name="${USER:-$(id -un)}"

    if command -v getent >/dev/null 2>&1; then
        group_exists=$(getent group docker || true)
    else
        group_exists=$(grep '^docker:' /etc/group 2>/dev/null || true)
    fi

    if [ -z "$group_exists" ]; then
        if command -v groupadd >/dev/null 2>&1; then
            run_as_root groupadd docker
        else
            run_as_root addgroup -S docker
        fi
    fi

    if ! id -nG "$user_name" | tr ' ' '\n' | grep -qx docker; then
        echo "Membership in the docker group grants root-equivalent access to this machine."
        if ! prompt_yes_no "Add $user_name to the docker group?"; then
            echo "Docker access is required to run Stilt without sudo."
            exit 1
        fi
        if command -v usermod >/dev/null 2>&1; then
            run_as_root usermod -aG docker "$user_name"
        else
            run_as_root addgroup "$user_name" docker
        fi
    fi

    reenter_docker_group
}

start_linux_docker_service() {
    if command -v systemctl >/dev/null 2>&1; then
        run_as_root systemctl enable --now docker
        return 0
    fi

    if command -v service >/dev/null 2>&1; then
        run_as_root service docker start
        return 0
    fi

    if command -v rc-service >/dev/null 2>&1; then
        run_as_root rc-service docker start
        return 0
    fi

    return 1
}

install_linux_docker() {
    if command -v yay >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            yay -S --noconfirm --needed docker-compose
        else
            yay -S --noconfirm --needed docker docker-compose
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v paru >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            paru -S --noconfirm --needed docker-compose
        else
            paru -S --noconfirm --needed docker docker-compose
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v pacman >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            run_as_root pacman -Sy --noconfirm docker-compose
        else
            run_as_root pacman -Sy --noconfirm docker docker-compose
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v apt-get >/dev/null 2>&1; then
        run_as_root apt-get update
        if command -v docker >/dev/null 2>&1; then
            run_as_root apt-get install -y docker-compose-plugin
        else
            run_as_root apt-get install -y docker.io docker-compose-plugin
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v dnf >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            run_as_root dnf install -y docker-compose-plugin
        else
            run_as_root dnf install -y docker docker-compose-plugin
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v yum >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            run_as_root yum install -y docker-compose-plugin
        else
            run_as_root yum install -y docker docker-compose-plugin
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v zypper >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            run_as_root zypper --non-interactive install docker-compose-plugin
        else
            run_as_root zypper --non-interactive install docker docker-compose-plugin
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v apk >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            run_as_root apk add docker-cli-compose
        else
            run_as_root apk add docker docker-cli-compose
        fi
        start_linux_docker_service || true
        return 0
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL https://get.docker.com | sh
        start_linux_docker_service || true
        return 0
    fi

    echo "No supported Linux installer was found."
    return 1
}

install_macos_docker() {
    if command -v brew >/dev/null 2>&1; then
        brew install --cask docker
        open -a Docker >/dev/null 2>&1 || true
        return 0
    fi

    echo "Homebrew is not installed."
    echo "Install Docker Desktop from https://www.docker.com/products/docker-desktop/ and start it once."
    return 1
}

install_windows_docker() {
    if command -v winget >/dev/null 2>&1; then
        winget install -e --id Docker.DockerDesktop
        return 0
    fi

    if command -v choco >/dev/null 2>&1; then
        choco install docker-desktop -y
        return 0
    fi

    if command -v powershell.exe >/dev/null 2>&1; then
        powershell.exe -NoProfile -Command "Start-Process 'https://www.docker.com/products/docker-desktop/'"
        echo "Install Docker Desktop from the browser, then start it once."
        return 1
    fi

    echo "Install Docker Desktop from https://www.docker.com/products/docker-desktop/."
    return 1
}

ensure_docker() {
    if docker_ready && compose_ready; then
        return 0
    fi

    if command -v docker >/dev/null 2>&1 && compose_ready; then
        docker_error=$(docker info 2>&1 || true)
        if [ "$(detect_platform)" = "linux" ] && printf '%s' "$docker_error" | grep -qi "permission denied"; then
            ensure_linux_docker_access
        fi

        if [ "$(detect_platform)" = "linux" ]; then
            start_linux_docker_service || true
            if docker_ready; then
                return 0
            fi
            docker_error=$(docker info 2>&1 || true)
            if printf '%s' "$docker_error" | grep -qi "permission denied"; then
                ensure_linux_docker_access
            fi
        fi
    fi

    if docker_ready && ! compose_ready; then
        echo "Docker found; Compose missing."
    else
        echo "Docker not found."
    fi

    if ! prompt_yes_no "Install now?"; then
        echo "Docker is required to run Stilt."
        exit 1
    fi

    case "$(detect_platform)" in
        linux)
            install_linux_docker
            ;;
        wsl|windows)
            install_windows_docker
            ;;
        macos)
            install_macos_docker
            ;;
        *)
            echo "Automatic installation is not supported on this system."
            exit 1
            ;;
    esac

    if ! wait_for_docker; then
        if [ "$(detect_platform)" = "linux" ]; then
            docker_error=$(docker info 2>&1 || true)
            if printf '%s' "$docker_error" | grep -qi "permission denied"; then
                ensure_linux_docker_access
            fi
        fi
        echo "Docker is installed, but not ready yet."
        exit 1
    fi
}

ensure_docker

plugins_file="plugins.conf"
services=$(awk -F= '/= enabled/ { gsub(/ /, "", $1); print $1 }' "$plugins_file")

mkdir -p logs
for svc in $services; do
    mkdir -p "data/$svc"
done

chmod -R 777 data

echo "Building..."
go build -o stilt ./cmd

echo "Generating config..."
./stilt

for image in $(docker compose -f docker-compose.yml config --images); do
    track_image "$image"
done

echo "Starting..."
docker compose -f docker-compose.yml up -d --force-recreate --remove-orphans

echo "Running."
docker compose ps --format "table {{.Service}}\t{{.Ports}}"
