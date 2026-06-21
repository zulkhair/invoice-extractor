#!/usr/bin/env python3
"""Benchmark harness (the decision tool).

Runs every candidate model over the labeled fixtures and prints field-level
accuracy (exact match per field, plus line-item precision/recall) and median
latency per model — which model gives the best accuracy/latency for your hardware.

    .venv/bin/python scripts/make_synthetic_fixture.py    # if you have no real ones yet
    .venv/bin/python scripts/bench.py
    .venv/bin/python scripts/bench.py --models qwen2.5vl:7b-q4_K_M

Fixtures: tests/fixtures/<stem>.{pdf,png,jpg,...} with ground truth in
tests/labels/<stem>.json. All candidates are compared on the VISION path (the
apples-to-apples axis), so digital PDFs are rasterized too.
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from app import config  # noqa: E402
from app.ollama_client import OllamaClient, OllamaError  # noqa: E402
from app.pipeline import parse_model_content  # noqa: E402
from app import prompts  # noqa: E402
from app.rasterize import image_to_png, is_image, pdf_to_pngs  # noqa: E402
from app.pdf_text import is_pdf  # noqa: E402
from app.schema import Invoice  # noqa: E402
from app.scoring import SCALAR_FIELDS, score_invoice  # noqa: E402

_IMG_EXT = {".png", ".jpg", ".jpeg", ".tif", ".tiff", ".webp", ".pdf"}


def _discover() -> list[tuple[str, Path, Path]]:
    out = []
    for f in sorted(config.FIXTURES_DIR.glob("*")):
        if f.suffix.lower() not in _IMG_EXT:
            continue
        label = config.LABELS_DIR / f"{f.stem}.json"
        if label.exists():
            out.append((f.stem, f, label))
    return out


def _to_images(path: Path) -> list[bytes]:
    data = path.read_bytes()
    if is_pdf(data):
        return pdf_to_pngs(data)
    if is_image(data):
        return [image_to_png(data)]
    raise ValueError(f"unsupported fixture: {path}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--models", nargs="*", default=config.BENCH_MODELS)
    args = ap.parse_args()

    fixtures = _discover()
    if not fixtures:
        print(f"No labeled fixtures in {config.FIXTURES_DIR} (+ {config.LABELS_DIR}).\n"
              "Add real invoices, or generate synthetic ones:\n"
              "  .venv/bin/python scripts/make_synthetic_fixture.py", file=sys.stderr)
        return 2

    client = OllamaClient()
    if not client.ping():
        print(f"Ollama not reachable at {config.OLLAMA_HOST} — start it first.", file=sys.stderr)
        return 2

    print(f"fixtures: {len(fixtures)}   models: {len(args.models)}   host: {config.OLLAMA_HOST}\n")
    sys_prompt = prompts.system_prompt()

    # per_model[model] = {accuracies, recalls, precisions, latencies, fails, field_hits}
    summaries = []
    for model in args.models:
        accs, recs, precs, lats = [], [], [], []
        fails = 0
        field_hits = {f: 0 for f in SCALAR_FIELDS}
        field_seen = {f: 0 for f in SCALAR_FIELDS}
        for stem, fpath, lpath in fixtures:
            gold = Invoice(**json.loads(lpath.read_text()))
            try:
                images = _to_images(fpath)
                res = client.chat(
                    model=model, system=sys_prompt,
                    user=prompts.vision_user_prompt(), images=images, json_format=True,
                )
                lats.append(res.latency_s)
                pred = parse_model_content(res.content)
            except (OllamaError, ValueError) as e:
                print(f"  [{model}] {stem}: ERROR {e}")
                fails += 1
                continue
            if pred is None:
                print(f"  [{model}] {stem}: parse failure (no schema-valid JSON)")
                fails += 1
                continue
            s = score_invoice(pred, gold)
            accs.append(s.accuracy)
            recs.append(s.line_item_recall)
            precs.append(s.line_item_precision)
            for f in SCALAR_FIELDS:
                field_seen[f] += 1
                field_hits[f] += int(s.per_field[f])
            print(f"  [{model}] {stem}: acc={s.accuracy:.2f} "
                  f"li_p={s.line_item_precision:.2f} li_r={s.line_item_recall:.2f} "
                  f"{res.latency_s:.1f}s")
        summaries.append({
            "model": model,
            "n": len(accs),
            "fails": fails,
            "acc": statistics.mean(accs) if accs else 0.0,
            "li_recall": statistics.mean(recs) if recs else 0.0,
            "li_precision": statistics.mean(precs) if precs else 0.0,
            "median_latency": statistics.median(lats) if lats else float("nan"),
            "field_hits": field_hits,
            "field_seen": field_seen,
        })

    # --- scorecard ---
    print("\n=== SCORECARD (field accuracy = mean over fixtures) ===")
    print(f"{'model':<34} {'n':>3} {'fail':>4} {'field_acc':>9} {'li_prec':>8} {'li_rec':>7} {'med_s':>7}")
    print("-" * 80)
    for s in summaries:
        print(f"{s['model']:<34} {s['n']:>3} {s['fails']:>4} {s['acc']:>9.3f} "
              f"{s['li_precision']:>8.3f} {s['li_recall']:>7.3f} {s['median_latency']:>7.1f}")

    # --- per-field breakdown ---
    print("\n=== PER-FIELD ACCURACY ===")
    header = "field".ljust(18) + "".join(f"{s['model'].split(':')[0][:12]:>14}" for s in summaries)
    print(header)
    print("-" * len(header))
    for f in SCALAR_FIELDS:
        row = f.ljust(18)
        for s in summaries:
            seen = s["field_seen"][f]
            row += f"{(s['field_hits'][f] / seen if seen else 0):>14.2f}"
        print(row)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
