#!/usr/bin/env python3
"""Build and validate the OpenXML conventions for the formal proposal DOCX."""

from __future__ import annotations

import argparse
import io
import os
import struct
import subprocess
import sys
import tempfile
import zipfile
import zlib
from pathlib import Path
from xml.etree import ElementTree as ET

from business_docx_contract import validate_contract


W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
PR = "http://schemas.openxmlformats.org/package/2006/relationships"
CT = "http://schemas.openxmlformats.org/package/2006/content-types"
NS = {"w": W, "r": R, "pr": PR, "ct": CT}

ET.register_namespace("w", W)
ET.register_namespace("r", R)


def qn(namespace: str, name: str) -> str:
    return f"{{{namespace}}}{name}"


def set_attr(element: ET.Element, name: str, value: str) -> None:
    element.set(qn(W, name), value)


def ensure(parent: ET.Element, name: str) -> ET.Element:
    child = parent.find(f"w:{name}", NS)
    return child if child is not None else ET.SubElement(parent, qn(W, name))


def replace_child(parent: ET.Element, name: str, attrs: dict[str, str]) -> ET.Element:
    old = parent.find(f"w:{name}", NS)
    if old is not None:
        parent.remove(old)
    child = ET.SubElement(parent, qn(W, name))
    for key, value in attrs.items():
        set_attr(child, key, value)
    return child


def read_package(path: Path) -> dict[str, bytes]:
    with zipfile.ZipFile(path) as archive:
        bad = archive.testzip()
        if bad:
            raise ValueError(f"DOCX ZIP 损坏：{bad}")
        return {name: archive.read(name) for name in archive.namelist()}


def write_package(path: Path, files: dict[str, bytes]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as tmp:
        tmp_path = Path(tmp.name)
    try:
        with zipfile.ZipFile(tmp_path, "w", zipfile.ZIP_DEFLATED) as archive:
            for name in sorted(files):
                archive.writestr(name, files[name])
        os.replace(tmp_path, path)
    finally:
        tmp_path.unlink(missing_ok=True)


def parse(files: dict[str, bytes], name: str) -> ET.Element:
    if name not in files:
        raise ValueError(f"DOCX 缺少 {name}")
    return ET.fromstring(files[name])


def serialize(root: ET.Element) -> bytes:
    root_namespace = root.tag.removeprefix("{").partition("}")[0]
    if root_namespace in (PR, CT):
        ET.register_namespace("", root_namespace)
    return ET.tostring(root, encoding="utf-8", xml_declaration=True)


def style(root: ET.Element, style_id: str, style_type: str = "paragraph") -> ET.Element:
    found = root.find(f"w:style[@w:styleId='{style_id}']", NS)
    if found is not None:
        return found
    found = ET.SubElement(root, qn(W, "style"))
    set_attr(found, "type", style_type)
    set_attr(found, "styleId", style_id)
    name = ET.SubElement(found, qn(W, "name"))
    set_attr(name, "val", style_id)
    return found


def set_run_style(
    style_element: ET.Element,
    east_asia: str,
    size: int,
    *,
    latin: str = "Times New Roman",
    bold: bool = False,
) -> None:
    rpr = ensure(style_element, "rPr")
    replace_child(rpr, "rFonts", {"ascii": latin, "hAnsi": latin, "eastAsia": east_asia})
    replace_child(rpr, "color", {"val": "000000"})
    replace_child(rpr, "sz", {"val": str(size)})
    replace_child(rpr, "szCs", {"val": str(size)})
    for name in ("b", "bCs"):
        old = rpr.find(f"w:{name}", NS)
        if old is not None:
            rpr.remove(old)
        if bold:
            ET.SubElement(rpr, qn(W, name))
    for name in ("i", "iCs"):
        replace_child(rpr, name, {"val": "0"})


def set_paragraph_style(
    style_element: ET.Element,
    *,
    before: int = 0,
    after: int = 0,
    line: int = 580,
    line_rule: str = "exact",
    align: str = "both",
    first_chars: int | None = None,
    keep_next: bool = False,
) -> None:
    ppr = ensure(style_element, "pPr")
    replace_child(ppr, "spacing", {"before": str(before), "after": str(after), "line": str(line), "lineRule": line_rule})
    replace_child(ppr, "jc", {"val": align})
    replace_child(ppr, "ind", {} if first_chars is None else {"firstLineChars": str(first_chars)})
    old = ppr.find("w:keepNext", NS)
    if old is not None:
        ppr.remove(old)
    if keep_next:
        ET.SubElement(ppr, qn(W, "keepNext"))


def configure_style(
    root: ET.Element,
    style_id: str,
    font: str,
    size: int,
    *,
    bold: bool = False,
    before: int = 0,
    after: int = 0,
    line: int = 580,
    line_rule: str = "exact",
    align: str = "both",
    first_chars: int | None = None,
    keep_next: bool = False,
) -> None:
    item = style(root, style_id)
    set_run_style(item, font, size, bold=bold)
    set_paragraph_style(item, before=before, after=after, line=line, line_rule=line_rule, align=align, first_chars=first_chars, keep_next=keep_next)


def configure_defaults(root: ET.Element) -> None:
    defaults = ensure(root, "docDefaults")
    rpr = ensure(ensure(defaults, "rPrDefault"), "rPr")
    replace_child(rpr, "rFonts", {"ascii": "Times New Roman", "hAnsi": "Times New Roman", "eastAsia": "仿宋_GB2312"})
    replace_child(rpr, "sz", {"val": "32"})
    replace_child(rpr, "szCs", {"val": "32"})
    ppr = ensure(ensure(defaults, "pPrDefault"), "pPr")
    replace_child(ppr, "spacing", {"before": "0", "after": "0", "line": "580", "lineRule": "exact"})


def configure_styles(root: ET.Element) -> None:
    configure_defaults(root)
    for style_id in ("Normal", "BodyText", "FirstParagraph"):
        configure_style(root, style_id, "仿宋_GB2312", 32, first_chars=200)
    configure_style(root, "Compact", "宋体", 24, line=360, align="left")
    configure_style(root, "Title", "方正小标宋简体", 44, before=1600, after=600, align="center")
    configure_style(root, "Subtitle", "楷体_GB2312", 32, before=300, after=300, align="center")
    configure_style(root, "Author", "方正小标宋简体", 32, before=2800, after=240, align="center")
    configure_style(root, "Date", "仿宋_GB2312", 32, align="center")
    configure_style(root, "Heading1", "黑体", 32, before=240, after=120, align="left", keep_next=True)
    configure_style(root, "Heading2", "楷体_GB2312", 32, bold=True, before=120, after=60, align="left", keep_next=True)
    for style_id in ("Heading3", "Heading4"):
        configure_style(root, style_id, "仿宋_GB2312", 32, bold=True, before=120, after=60, align="left", keep_next=True)
    configure_style(root, "TOCHeading", "方正小标宋简体", 44, before=240, after=240, align="center")
    for level, indent in ((1, 0), (2, 240), (3, 480)):
        configure_style(root, f"TOC{level}", "仿宋_GB2312", 28, line=420, align="left")
        replace_child(ensure(style(root, f"TOC{level}"), "pPr"), "ind", {"left": str(indent)})
    configure_style(root, "ImageCaption", "宋体", 21, after=120, line=300, align="center")
    configure_style(root, "TableCaption", "宋体", 21, after=60, line=300, align="center", keep_next=True)
    configure_style(root, "CaptionedFigure", "宋体", 21, line=300, line_rule="atLeast", align="center", keep_next=True)


def section_properties(section: ET.Element, *, next_page: bool, footer: bool, page_start: bool) -> None:
    for child in list(section):
        section.remove(child)
    if footer:
        reference = ET.SubElement(section, qn(W, "footerReference"))
        set_attr(reference, "type", "default")
        reference.set(qn(R, "id"), "rIdBusinessFooter")
    if next_page:
        replace_child(section, "type", {"val": "nextPage"})
    replace_child(section, "pgSz", {"w": "11906", "h": "16838"})
    replace_child(section, "pgMar", {"top": "2098", "right": "1474", "bottom": "1984", "left": "1587", "header": "850", "footer": "992", "gutter": "0"})
    if page_start:
        replace_child(section, "pgNumType", {"start": "1", "fmt": "decimal"})
    replace_child(section, "docGrid", {"type": "lines", "linePitch": "580"})


def paragraph_style(element: ET.Element) -> str:
    style_element = element.find("w:pPr/w:pStyle", NS)
    return "" if style_element is None else style_element.get(qn(W, "val"), "")


def add_section_to_paragraph(paragraph: ET.Element, *, next_page: bool) -> None:
    ppr = ensure(paragraph, "pPr")
    section = ensure(ppr, "sectPr")
    section_properties(section, next_page=next_page, footer=False, page_start=False)


def configure_sections(root: ET.Element) -> None:
    body = root.find("w:body", NS)
    if body is None:
        raise ValueError("document.xml 缺少 w:body")
    children = list(body)
    date = next((item for item in children if item.tag == qn(W, "p") and paragraph_style(item) == "Date"), None)
    toc = next((item for item in children if item.tag == qn(W, "sdt")), None)
    if date is None or toc is None:
        raise ValueError("无法定位封面日期或目录结构")
    add_section_to_paragraph(date, next_page=True)
    toc_break = ET.Element(qn(W, "p"))
    add_section_to_paragraph(toc_break, next_page=True)
    body.insert(list(body).index(toc) + 1, toc_break)
    final = body.find("w:sectPr", NS)
    if final is None:
        final = ET.SubElement(body, qn(W, "sectPr"))
    section_properties(final, next_page=False, footer=True, page_start=True)


def footer_xml() -> bytes:
    footer = ET.Element(qn(W, "ftr"))
    paragraph = ET.SubElement(footer, qn(W, "p"))
    ppr = ET.SubElement(paragraph, qn(W, "pPr"))
    replace_child(ppr, "jc", {"val": "center"})
    for kind, value in (("text", "— "), ("begin", ""), ("instr", " PAGE "), ("separate", ""), ("text", "1"), ("end", ""), ("text", " —")):
        run = ET.SubElement(paragraph, qn(W, "r"))
        if kind == "text":
            text = ET.SubElement(run, qn(W, "t"))
            text.text = value
        elif kind == "instr":
            instr = ET.SubElement(run, qn(W, "instrText"))
            instr.set("{http://www.w3.org/XML/1998/namespace}space", "preserve")
            instr.text = value
        else:
            field = ET.SubElement(run, qn(W, "fldChar"))
            set_attr(field, "fldCharType", kind)
    return serialize(footer)


def configure_footer(files: dict[str, bytes]) -> None:
    rels = parse(files, "word/_rels/document.xml.rels")
    if not any(item.get("Id") == "rIdBusinessFooter" for item in rels):
        ET.SubElement(rels, qn(PR, "Relationship"), {"Id": "rIdBusinessFooter", "Type": f"{R}/footer", "Target": "footer1.xml"})
    content = parse(files, "[Content_Types].xml")
    if not any(item.get("PartName") == "/word/footer1.xml" for item in content):
        ET.SubElement(content, qn(CT, "Override"), {"PartName": "/word/footer1.xml", "ContentType": "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"})
    files["word/_rels/document.xml.rels"] = serialize(rels)
    files["[Content_Types].xml"] = serialize(content)
    files["word/footer1.xml"] = footer_xml()


def configure_settings(files: dict[str, bytes]) -> None:
    root = parse(files, "word/settings.xml")
    update = root.find("w:updateFields", NS)
    if update is None:
        update = ET.SubElement(root, qn(W, "updateFields"))
    set_attr(update, "val", "true")
    files["word/settings.xml"] = serialize(root)


def configure_tables(root: ET.Element) -> None:
    for table in root.findall(".//w:tbl", NS):
        properties = ensure(table, "tblPr")
        replace_child(properties, "tblW", {"w": "5000", "type": "pct"})
        rows = table.findall("w:tr", NS)
        for index, row in enumerate(rows):
            row_properties = ensure(row, "trPr")
            if index == 0 and row_properties.find("w:tblHeader", NS) is None:
                ET.SubElement(row_properties, qn(W, "tblHeader"))
            if row_properties.find("w:cantSplit", NS) is None:
                ET.SubElement(row_properties, qn(W, "cantSplit"))


def build_reference(output: Path) -> None:
    result = subprocess.run(["pandoc", "--print-default-data-file", "reference.docx"], check=True, stdout=subprocess.PIPE)
    with tempfile.NamedTemporaryFile(suffix=".docx") as tmp:
        Path(tmp.name).write_bytes(result.stdout)
        files = read_package(Path(tmp.name))
    styles = parse(files, "word/styles.xml")
    configure_styles(styles)
    document = parse(files, "word/document.xml")
    final = document.find("w:body/w:sectPr", NS)
    if final is not None:
        section_properties(final, next_page=False, footer=False, page_start=False)
    files["word/styles.xml"] = serialize(styles)
    files["word/document.xml"] = serialize(document)
    configure_settings(files)
    write_package(output, files)


def finalize(input_path: Path, output: Path) -> None:
    files = read_package(input_path)
    document = parse(files, "word/document.xml")
    configure_sections(document)
    configure_tables(document)
    files["word/document.xml"] = serialize(document)
    configure_footer(files)
    configure_settings(files)
    write_package(output, files)


def enable_update_fields(path: Path) -> None:
    files = read_package(path)
    configure_settings(files)
    write_package(path, files)


def png_dpi(path: Path) -> None:
    data = path.read_bytes()
    if not data.startswith(b"\x89PNG\r\n\x1a\n"):
        raise ValueError(f"不是 PNG：{path}")
    chunks, offset = [], 8
    while offset < len(data):
        length = struct.unpack(">I", data[offset : offset + 4])[0]
        chunk_type = data[offset + 4 : offset + 8]
        end = offset + 12 + length
        if chunk_type != b"pHYs":
            chunks.append((chunk_type, data[offset + 8 : offset + 8 + length]))
        offset = end
    output = bytearray(data[:8])
    for index, (chunk_type, payload) in enumerate(chunks):
        output.extend(png_chunk(chunk_type, payload))
        if index == 0:
            output.extend(png_chunk(b"pHYs", struct.pack(">IIB", 11811, 11811, 1)))
    path.write_bytes(output)


def png_chunk(chunk_type: bytes, payload: bytes) -> bytes:
    checksum = zlib.crc32(chunk_type + payload) & 0xFFFFFFFF
    return struct.pack(">I", len(payload)) + chunk_type + payload + struct.pack(">I", checksum)


def package_text(files: dict[str, bytes], name: str) -> str:
    return files.get(name, b"").decode("utf-8", errors="replace")


def validate(path: Path) -> None:
    files = read_package(path)
    document = parse(files, "word/document.xml")
    sections = document.findall(".//w:sectPr", NS)
    if len(sections) != 3:
        raise ValueError(f"分节数量应为 3，实际为 {len(sections)}")
    for section in sections:
        size = section.find("w:pgSz", NS)
        margin = section.find("w:pgMar", NS)
        if size is None or size.get(qn(W, "w")) != "11906" or size.get(qn(W, "h")) != "16838":
            raise ValueError("存在非 A4 分节")
        expected = {"top": "2098", "right": "1474", "bottom": "1984", "left": "1587"}
        if margin is None or any(margin.get(qn(W, key)) != value for key, value in expected.items()):
            raise ValueError("页边距不符合公文版心")
    validate_contract(files, document, sections)
    validate_content(files, document, sections[-1])


def validate_content(files: dict[str, bytes], document: ET.Element, final: ET.Element) -> None:
    if final.find("w:footerReference", NS) is None or final.find("w:pgNumType", NS) is None:
        raise ValueError("正文页脚或起始页码缺失")
    document_text = "".join(document.itertext())
    toc = document.find(".//w:sdt", NS)
    toc_text = "" if toc is None else "".join(toc.itertext())
    instructions = "".join(item.text or "" for item in document.findall(".//w:instrText", NS))
    if "TOC" not in instructions or "\\o" not in instructions or "目　录" not in toc_text:
        raise ValueError("中文目录或 TOC 字段缺失")
    if "一、项目摘要" not in toc_text:
        raise ValueError("中文目录没有生成正文条目")
    if any(field in document_text for field in ("title:", "subtitle:", "lang:", "status:")):
        raise ValueError("YAML 元数据泄漏到正文")
    media = [name for name in files if name.startswith("word/media/")]
    if len([name for name in media if name.lower().endswith(".png")]) != 4:
        raise ValueError("必须恰好嵌入 4 张 PNG 技术路线图")
    if any(name.lower().endswith(".svg") for name in media):
        raise ValueError("DOCX 不得嵌入 SVG")
    styles = package_text(files, "word/styles.xml")
    if not all(font in styles for font in ("方正小标宋简体", "仿宋_GB2312", "Times New Roman")):
        raise ValueError("中文或西文字体声明不完整")
    settings = parse(files, "word/settings.xml")
    update = settings.find("w:updateFields", NS)
    if update is None or update.get(qn(W, "val")) != "true":
        raise ValueError("未启用目录和页码字段更新")


def main() -> int:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)
    reference = subparsers.add_parser("reference")
    reference.add_argument("--output", type=Path, required=True)
    final = subparsers.add_parser("finalize")
    final.add_argument("--input", type=Path, required=True)
    final.add_argument("--output", type=Path, required=True)
    check = subparsers.add_parser("validate")
    check.add_argument("path", type=Path)
    fields = subparsers.add_parser("update-fields")
    fields.add_argument("path", type=Path)
    dpi = subparsers.add_parser("png-dpi")
    dpi.add_argument("path", type=Path)
    args = parser.parse_args()
    if args.command == "reference":
        build_reference(args.output)
    elif args.command == "finalize":
        finalize(args.input, args.output)
    elif args.command == "validate":
        validate(args.path)
    elif args.command == "update-fields":
        enable_update_fields(args.path)
    else:
        png_dpi(args.path)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, ET.ParseError, subprocess.CalledProcessError, zipfile.BadZipFile) as error:
        print(f"[business-docx][ERROR] {error}", file=sys.stderr)
        raise SystemExit(1)
