#!/usr/bin/env python3
"""Task 2 — vision sanity check (critical).

Feed each candidate model an actual invoice IMAGE and ask it to read a value off
the page. A model that returns the right value AND returns a *different* value for
a different control image is genuinely reading pixels. A model that's right on one
but identical across both has a broken/ignored vision projector -> unusable.

    .venv/bin/python scripts/make_synthetic_fixture.py     # once, to create the image
    .venv/bin/python scripts/vision_check.py
    .venv/bin/python scripts/vision_check.py --image path/to/real_invoice.png --expect INV-123
"""

from __future__ import annotations

import argparse
import io
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from app import config  # noqa: E402
from app.ollama_client import OllamaClient, OllamaError  # noqa: E402

CONTROL_TOKEN = "CTRL-7788-XYZ"
_DEFAULT_IMAGE = config.FIXTURES_DIR / "synthetic_invoice_scan.png"
_DEFAULT_EXPECT = "INV/2026/06/0042"


def _alnum(s: str) -> str:
    return re.sub(r"[^A-Za-z0-9]", "", s).upper()


def _control_image() -> bytes:
    """A distinct image whose invoice number is CONTROL_TOKEN."""
    from PIL import Image, ImageDraw, ImageFont

    img = Image.new("RGB", (900, 500), "white")
    d = ImageDraw.Draw(img)
    try:
        font = ImageFont.load_default(size=40)
    except TypeError:  # very old Pillow
        font = ImageFont.load_default()
    d.text((40, 60), "CONTROL INVOICE", fill="black", font=font)
    d.text((40, 160), f"Invoice No: {CONTROL_TOKEN}", fill="black", font=font)
    d.text((40, 260), "Total: 1.234.567", fill="black", font=font)
    out = io.BytesIO()
    img.save(out, format="PNG")
    return out.getvalue()


def _ask(client: OllamaClient, model: str, image: bytes) -> str:
    res = client.chat(
        model=model,
        system="You read text off invoice images. Answer with only what is asked.",
        user="What is the invoice number printed on this image? Reply with ONLY the invoice number.",
        images=[image],
        json_format=False,
        temperature=0,
    )
    return res.content.strip()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--image", default=str(_DEFAULT_IMAGE))
    ap.add_argument("--expect", default=_DEFAULT_EXPECT)
    ap.add_argument("--models", nargs="*", default=config.BENCH_MODELS)
    args = ap.parse_args()

    img_path = Path(args.image)
    if not img_path.exists():
        print(f"image not found: {img_path}\n"
              "run: .venv/bin/python scripts/make_synthetic_fixture.py", file=sys.stderr)
        return 2

    main_img = img_path.read_bytes()
    ctrl_img = _control_image()
    expect = _alnum(args.expect)
    client = OllamaClient()

    if not client.ping():
        print(f"Ollama not reachable at {config.OLLAMA_HOST} — start it first.", file=sys.stderr)
        return 2

    print(f"image:  {img_path}")
    print(f"expect: {args.expect}\n")
    print(f"{'model':<34} {'reads?':<7} {'distinct?':<10} verdict")
    print("-" * 72)

    any_usable = False
    for model in args.models:
        try:
            a_main = _ask(client, model, main_img)
            a_ctrl = _ask(client, model, ctrl_img)
        except OllamaError as e:
            print(f"{model:<34} ERROR: {e}")
            continue
        reads = expect in _alnum(a_main)
        ctrl_ok = _alnum(CONTROL_TOKEN) in _alnum(a_ctrl)
        distinct = _alnum(a_main) != _alnum(a_ctrl)
        usable = reads and distinct
        any_usable = any_usable or usable
        verdict = "USABLE" if usable else ("BROKEN-PROJECTOR" if reads and not distinct else "FAIL")
        print(f"{model:<34} {str(reads):<7} {str(distinct):<10} {verdict}"
              f"   main={a_main!r} ctrl_ok={ctrl_ok}")

    print()
    if any_usable:
        print("PASS: at least one model demonstrably reads pixels. Record it as the vision model.")
        return 0
    print("FAIL: no model reliably read the image. Do not proceed with the vision path.")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
