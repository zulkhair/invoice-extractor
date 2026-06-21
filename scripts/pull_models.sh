#!/usr/bin/env bash
# Verify each model tag exists in the Ollama registry BEFORE pulling, refuse
# FP16/BF16 variants (hard rule: older GPUs run FP16 far slower than FP32), and pull the rest.
#
#   bash scripts/pull_models.sh
#
# Verification hits the public tags page (ollama.com/.../tags) and greps for the
# exact variant — if a tag 404s or the variant is gone, it's reported and skipped
# so you can find the current equivalent (per spec hard rule).
set -uo pipefail

if ! command -v ollama >/dev/null 2>&1; then
  echo "ollama not installed — run: bash scripts/setup_env.sh" >&2
  exit 1
fi

# Load tags from a per-app .env if present (python/.env then go/.env), else the
# verified defaults. Models are identical for both apps.
load() { grep -hE "^$1=" python/.env go/.env 2>/dev/null | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*//; s/^[[:space:]]*//; s/[[:space:]]*$//'; }

MODELS=(
  "$(load MODEL_SMALL    || true)"
  "$(load MODEL_PRIMARY  || true)"
  "$(load MODEL_FALLBACK || true)"
)
# defaults (verified 2026-06-21) if .env is absent
[ -z "${MODELS[0]}" ] && MODELS[0]="qwen2.5vl:3b-q4_K_M"
[ -z "${MODELS[1]}" ] && MODELS[1]="frob/nuextract-2.0:8b-q4_K_M"
[ -z "${MODELS[2]}" ] && MODELS[2]="qwen2.5vl:7b-q4_K_M"

tags_url() {  # tag -> tags page URL + the variant we must find
  local tag="$1" name variant
  name="${tag%%:*}"; variant="${tag#*:}"; [ "$variant" = "$tag" ] && variant="latest"
  if [[ "$name" == */* ]]; then echo "https://ollama.com/${name}/tags|$variant"
  else echo "https://ollama.com/library/${name}/tags|$variant"; fi
}

is_fp16() { [[ "$1" =~ fp16|bf16|-f16 ]]; }

fail=0
for tag in "${MODELS[@]}"; do
  echo "== $tag"
  if is_fp16 "$tag"; then
    echo "   REFUSED: FP16/BF16 variant violates the quantized-only rule. Skipping."
    fail=1; continue
  fi
  IFS='|' read -r url variant <<<"$(tags_url "$tag")"
  if curl -fsS -m 15 "$url" 2>/dev/null | grep -q -- "$variant"; then
    echo "   verified ($variant present at $url) -> pulling"
    if ollama pull "$tag"; then echo "   pulled"; else echo "   PULL FAILED"; fail=1; fi
  else
    echo "   NOT FOUND on registry ($url has no '$variant')."
    echo "   Find the current GGUF equivalent and update .env / README. Skipping."
    fail=1
  fi
done

echo
echo "== ollama list"
ollama list || true

[ "$fail" -eq 0 ] && echo "All models verified and pulled." \
                  || echo "Some models were skipped — see notes above."
exit "$fail"
