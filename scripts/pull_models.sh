#!/usr/bin/env bash
# Pull the model tags, refusing FP16/BF16 variants (older GPUs run FP16 far
# slower than FP32). `ollama pull` is the verifier: it checks the registry and
# fails fast on a missing tag (manifest 404) before downloading any blob.
#
#   bash scripts/pull_models.sh
set -uo pipefail

# Run from the repo root regardless of where this is invoked, so .env lookups work.
cd "$(dirname "$0")/.." || exit 1

if ! command -v ollama >/dev/null 2>&1; then
  echo "ollama not installed — run: bash scripts/setup_env.sh" >&2
  exit 1
fi

# Load tags from a per-app .env if present (python/.env then go/.env), else the
# defaults below. Models are identical for both apps.
load() { grep -hE "^$1=" python/.env go/.env 2>/dev/null | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*//; s/^[[:space:]]*//; s/[[:space:]]*$//'; }

MODELS=(
  "$(load MODEL_SMALL    || true)"
  "$(load MODEL_PRIMARY  || true)"
  "$(load MODEL_FALLBACK || true)"
)
[ -z "${MODELS[0]}" ] && MODELS[0]="qwen2.5vl:3b-q4_K_M"
[ -z "${MODELS[1]}" ] && MODELS[1]="frob/nuextract-2.0:8b-q4_K_M"
[ -z "${MODELS[2]}" ] && MODELS[2]="qwen2.5vl:7b-q4_K_M"

is_fp16() { [[ "$1" =~ fp16|bf16|-f16 ]]; }

fail=0
for tag in "${MODELS[@]}"; do
  echo "== $tag"
  if is_fp16 "$tag"; then
    echo "   REFUSED: FP16/BF16 variant violates the quantized-only rule. Skipping."
    fail=1; continue
  fi
  if ollama pull "$tag"; then
    echo "   ok"
  else
    echo "   FAILED — tag may not exist. Find the current GGUF and update .env / README."
    fail=1
  fi
done

echo
echo "== ollama list"
ollama list || true

[ "$fail" -eq 0 ] && echo "All models pulled." \
                  || echo "Some models failed — see notes above."
exit "$fail"
