#!/usr/bin/env python3
"""Negative package tests for DOCX relationship validation."""

from __future__ import annotations

import subprocess
import sys
import tempfile
from pathlib import Path
from xml.etree import ElementTree as ET


ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "tools/docs"))

import business_docx as docx  # noqa: E402


def mutate_footer(files: dict[str, bytes]) -> None:
    rels = docx.parse(files, "word/_rels/document.xml.rels")
    relationship = next(item for item in rels if item.get("Type") == f"{docx.R}/footer")
    relationship.set("Target", "footer2.xml")
    files["word/_rels/document.xml.rels"] = docx.serialize(rels)
    files["word/footer2.xml"] = (
        b'<?xml version="1.0" encoding="UTF-8"?>'
        b'<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
        b"<w:p><w:r><w:t>blank</w:t></w:r></w:p></w:ftr>"
    )


def mutate_image_relationships(files: dict[str, bytes]) -> None:
    rels = docx.parse(files, "word/_rels/document.xml.rels")
    for relationship in list(rels):
        if relationship.get("Type") == f"{docx.R}/image":
            rels.remove(relationship)
    files["word/_rels/document.xml.rels"] = docx.serialize(rels)


def expect_rejected(source: Path, name: str, mutation) -> None:
    files = docx.read_package(source)
    mutation(files)
    with tempfile.TemporaryDirectory() as directory:
        candidate = Path(directory) / f"{name}.docx"
        docx.write_package(candidate, files)
        result = subprocess.run(
            [sys.executable, str(ROOT / "tools/docs/business_docx.py"), "validate", str(candidate)],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    if result.returncode == 0:
        raise AssertionError(f"校验器错误放行负向样本：{name}")


def main() -> int:
    source = Path(sys.argv[1])
    expect_rejected(source, "blank-footer-target", mutate_footer)
    expect_rejected(source, "missing-image-relationships", mutate_image_relationships)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
