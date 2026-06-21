#!/usr/bin/env python3
"""Write the synthetic invoice fixture (PDF + scan PNG + labels) into BOTH apps'
test dirs, so the vision check and benchmark have something to run on without real
invoices. Sharing one fixture keeps Python and Go benchmarks comparable.

    python scripts/make_synthetic_fixture.py
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from app import config  # noqa: E402
from tests._synthetic import build_synthetic  # noqa: E402

# python/ is config.PROJECT_ROOT; the repo root is its parent.
_REPO_ROOT = config.PROJECT_ROOT.parent
_GO_TESTDATA = _REPO_ROOT / "go" / "testdata"

TARGETS = [
    (config.FIXTURES_DIR, config.LABELS_DIR),                 # python/tests/...
    (_GO_TESTDATA / "fixtures", _GO_TESTDATA / "labels"),     # go/testdata/...
]


def main() -> None:
    for fixtures_dir, labels_dir in TARGETS:
        paths = build_synthetic(fixtures_dir, labels_dir)
        for name, p in paths.items():
            print(f"  {name}: {p}")
        print(f"  labels: {labels_dir}\n")


if __name__ == "__main__":
    main()
