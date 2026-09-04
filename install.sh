#!/usr/bin/env bash
# Rechvix — one-command self-hosted install.
#
#   curl -fsSL https://raw.githubusercontent.com/Raktim94/rechvix/main/install.sh | bash
#
# What this does: clones the repo (if not already inside it), generates the
# secrets deploy/compose/.env needs, and runs `docker compose up -d`. It
# does not touch anything outside the clone directory it creates/uses.
#
# Requires: bash, curl, git, and Docker (Engine + Compose v2 plugin, or
# Docker Desktop on macOS). Linux and macOS only — see the README/CONTRIBUTING
# for Windows (WSL2) guidance; there is no native Windows path.
set -euo pipefail

REPO_URL="https://github.com/Raktim94/rechvix.git"
INSTALL_DIR="${RECHVIX_INSTALL_DIR:-rechvix}"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$1" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required but not found on PATH. $2"
}

require_cmd git   "Install git: https://git-scm.com/downloads"
require_cmd curl  "Install curl (usually already present on Linux/macOS)."
require_cmd docker "Install Docker: https://docs.docker.com/get-docker/ (macOS: Docker Desktop; Linux: Docker Engine)."

if ! docker compose version >/dev/null 2>&1; then
  if command -v docker-compose >/dev/null 2>&1; then
    warn "Using legacy 'docker-compose' — consider upgrading to the 'docker compose' plugin (Compose v2)."
    COMPOSE="docker-compose"
  else
    die "Docker Compose not found. Install Docker Desktop, or the 'docker compose' plugin: https://docs.docker.com/compose/install/"
  fi
else
  COMPOSE="docker compose"
fi

if ! docker info >/dev/null 2>&1; then
  die "Docker is installed but not running (or you lack permission). Start Docker and, on Linux, ensure your user is in the 'docker' group."
fi

# --- get the source ---
if [ -f "go.mod" ] && grep -q '^module rechvix' go.mod 2>/dev/null; then
  log "Already inside the rechvix repo — using the current directory."
  REPO_DIR="$(pwd)"
elif [ -d "$INSTALL_DIR/.git" ]; then
  log "Found existing clone at ./$INSTALL_DIR — pulling latest main."
  git -C "$INSTALL_DIR" pull --ff-only
  REPO_DIR="$(pwd)/$INSTALL_DIR"
else
  log "Cloning $REPO_URL into ./$INSTALL_DIR ..."
  git clone --depth 1 "$REPO_URL" "$INSTALL_DIR"
  REPO_DIR="$(pwd)/$INSTALL_DIR"
fi

cd "$REPO_DIR/deploy/compose"

# --- generate .env if it doesn't exist yet ---
if [ -f .env ]; then
  log ".env already exists — leaving it untouched. Delete it first if you want fresh secrets."
else
  log "Generating deploy/compose/.env with fresh secrets ..."
  gen_secret() { openssl rand -base64 24 2>/dev/null || head -c 24 /dev/urandom | base64; }
  gen_key()    { openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64; }

  cp .env.example .env
  # macOS/BSD sed needs `-i ''`; GNU sed needs `-i` with no argument — detect.
  SED_INPLACE=(-i)
  if [[ "$(uname)" == "Darwin" ]]; then SED_INPLACE=(-i ''); fi

  sed "${SED_INPLACE[@]}" "s#^POSTGRES_PASSWORD=.*#POSTGRES_PASSWORD=$(gen_secret)#" .env
  sed "${SED_INPLACE[@]}" "s#^BILLING_APP_PASSWORD=.*#BILLING_APP_PASSWORD=$(gen_secret)#" .env
  sed "${SED_INPLACE[@]}" "s#^AEAD_ENCRYPTION_KEY=.*#AEAD_ENCRYPTION_KEY=$(gen_key)#" .env
fi

HTTP_PORT="$(grep -E '^HTTP_PORT=' .env | cut -d= -f2)"
HTTP_PORT="${HTTP_PORT:-8080}"

log "Starting Rechvix (postgres, migrate, app, worker) via $COMPOSE ..."
$COMPOSE up -d --build

log "Done. Open http://localhost:${HTTP_PORT}/setup to create your organisation."
log "Secrets and settings live in: $REPO_DIR/deploy/compose/.env"
log "Logs: cd $REPO_DIR/deploy/compose && $COMPOSE logs -f"
