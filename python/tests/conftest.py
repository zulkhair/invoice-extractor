"""Shared pytest fixtures. Generates the synthetic invoice on demand so document
tests are self-contained (no committed binaries required)."""

from pathlib import Path

import pytest

from app import config
from tests._synthetic import LABEL, build_synthetic


@pytest.fixture(scope="session")
def synthetic(tmp_path_factory):
    out = tmp_path_factory.mktemp("synthetic")
    paths = build_synthetic(out / "fixtures", out / "labels")
    return {**paths, "label": LABEL}


@pytest.fixture(scope="session")
def synthetic_pdf_bytes(synthetic) -> bytes:
    return Path(synthetic["pdf"]).read_bytes()


@pytest.fixture(scope="session")
def synthetic_png_bytes(synthetic) -> bytes:
    return Path(synthetic["png"]).read_bytes()
