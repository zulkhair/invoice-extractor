#!/usr/bin/env bash
# Provision the host for BOTH the Python and Go invoice extractors on a fresh
# Linux/WSL2 box. Installs the SHARED system deps (Ollama + poppler) —
# these need sudo, so run this yourself. Per-language setup is below.
#
#   bash scripts/setup_env.sh
set -euo pipefail

echo "==> GPU visibility (WSL2 passthrough)"
if command -v nvidia-smi >/dev/null 2>&1; then
  nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader || true
else
  echo "!! nvidia-smi not found. Install the Windows NVIDIA driver for WSL2 GPU access."
fi

echo
echo "==> poppler-utils (pdftotext/pdftoppm — used by Python pdf2image AND the Go poppler backend)"
if command -v pdftoppm >/dev/null 2>&1; then
  echo "   already installed"
else
  sudo apt-get update -qq && sudo apt-get install -y poppler-utils
fi

echo
echo "==> Ollama"
if command -v ollama >/dev/null 2>&1; then
  echo "   already installed: $(ollama --version 2>&1 | head -1)"
else
  curl -fsSL https://ollama.com/install.sh | sh
fi

echo
echo "==> Ollama server"
if curl -fsS -m 3 http://127.0.0.1:11434/api/tags >/dev/null 2>&1; then
  echo "   already responding on :11434"
else
  echo "   launching 'ollama serve' (log: /tmp/ollama.log)"
  nohup ollama serve >/tmp/ollama.log 2>&1 &
  sleep 3
  curl -fsS -m 5 http://127.0.0.1:11434/api/tags >/dev/null 2>&1 \
    && echo "   up" || echo "   !! did not come up — check /tmp/ollama.log"
fi

cat <<'EOF'

==> Next steps
  Shared:
    bash scripts/pull_models.sh                 # verify + pull models (Task 1)
    python/.venv/bin/python python/scripts/make_synthetic_fixture.py   # writes fixtures to BOTH apps

  Python app (python/):
    cd python
    python3 -m venv .venv && .venv/bin/pip install -e ".[dev]"
    # if 'ensurepip is not available':
    #   python3 -m venv --without-pip .venv
    #   curl -fsSL https://bootstrap.pypa.io/get-pip.py | .venv/bin/python -
    #   .venv/bin/pip install -e ".[dev]"
    cp .env.example .env
    .venv/bin/uvicorn app.main:app --reload     # /health, /extract
    .venv/bin/python scripts/vision_check.py     # Task 2

  Go app (go/):
    cd go
    cp .env.example .env
    go run ./cmd/server                          # /health, /extract
    go run ./cmd/visioncheck                      # Task 2
    go run ./cmd/bench                             # Task 7
EOF
