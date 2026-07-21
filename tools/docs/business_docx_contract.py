"""Validate the delivery-level OpenXML contract for the proposal DOCX."""

from __future__ import annotations

import struct
import posixpath
from xml.etree import ElementTree as ET


W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
PR = "http://schemas.openxmlformats.org/package/2006/relationships"
A = "http://schemas.openxmlformats.org/drawingml/2006/main"
NS = {"w": W, "pr": PR}

COVER_LINES = (
    "SysArmor 主机安全技术研究及平台建设项目建议书",
    "面向政府与国有企业场景",
    "项目建议书（讨论稿）",
    "二〇二六年七月",
)
TABLE_CAPTIONS = (
    "表 1　现有方案能力边界",
    "表 2　当前能力边界",
    "表 3　实施计划",
    "表 4　考核指标",
    "表 5　业务概念与工程名称对照",
)
FIGURE_CAPTIONS = (
    "图 1　动态博弈通过统一策略控制平面形成受约束的持续调整闭环。",
    "图 2　端侧负责低时延、离线可用的安全能力，中心侧在授权边界内扩展关联和管理能力。",
    "图 3　安全信息价值逐级提高、数据规模逐级降低，材料不足时只能受控补充观察。",
    "图 4　项目按照关键问题、研究内容、研究方法、分层验证和预期成果形成完整论证链。",
)


def qn(namespace: str, name: str) -> str:
    return f"{{{namespace}}}{name}"


def element_text(element: ET.Element) -> str:
    return "".join(element.itertext())


def paragraph_style(paragraph: ET.Element) -> str:
    style = paragraph.find("w:pPr/w:pStyle", NS)
    return "" if style is None else style.get(qn(W, "val"), "")


def validate_cover_and_headings(document: ET.Element) -> None:
    body = document.find("w:body", NS)
    if body is None:
        raise ValueError("document.xml 缺少 w:body")
    cover = tuple(element_text(item) for item in list(body) if item.tag == qn(W, "p"))[:4]
    if cover != COVER_LINES:
        raise ValueError("中性封面四行文案不符合设计规格")
    paragraphs = document.findall(".//w:p", NS)
    styles = {element_text(item): paragraph_style(item) for item in paragraphs}
    if styles.get("一、项目摘要") != "Heading1":
        raise ValueError("正文一级标题未使用 Heading1")
    principle = "动态博弈：使防御能力能够受控调整"
    if styles.get(principle) != "Heading3":
        raise ValueError("正文三级标题未使用 Heading3")
    toc = document.find(".//w:sdt", NS)
    if toc is None or principle not in element_text(toc):
        raise ValueError("中文目录未收录三级标题")


def validate_table_captions(document: ET.Element) -> None:
    captions = [
        element_text(item)
        for item in document.findall(".//w:p", NS)
        if paragraph_style(item) == "TableCaption"
    ]
    if tuple(captions) != TABLE_CAPTIONS:
        raise ValueError("五张核心表的表题、顺序或编号不正确")
    body = document.find("w:body", NS)
    children = [] if body is None else list(body)
    caption_indexes = [index for index, item in enumerate(children) if paragraph_style(item) == "TableCaption"]
    if len(caption_indexes) != 5 or any(index + 1 >= len(children) or children[index + 1].tag != qn(W, "tbl") for index in caption_indexes):
        raise ValueError("表题必须紧邻对应表格并置于表格上方")


def validate_figure_captions(document: ET.Element) -> None:
    captions = [
        element_text(item)
        for item in document.findall(".//w:p", NS)
        if paragraph_style(item) == "ImageCaption"
    ]
    if tuple(captions) != FIGURE_CAPTIONS:
        raise ValueError("四张技术路线图的图题、顺序或编号不正确")


def validate_sections(files: dict[str, bytes], sections: list[ET.Element]) -> None:
    for section in sections:
        grid = section.find("w:docGrid", NS)
        if grid is None or grid.get(qn(W, "linePitch")) != "580":
            raise ValueError("文档网格必须按约 22 行配置")
    for section in sections[:2]:
        if section.find("w:headerReference", NS) is not None or section.find("w:footerReference", NS) is not None:
            raise ValueError("封面或目录不得包含页眉页脚")
    final = sections[-1]
    if final.find("w:headerReference", NS) is not None:
        raise ValueError("正文不得包含页眉")
    page_number = final.find("w:pgNumType", NS)
    if page_number is None or page_number.get(qn(W, "start")) != "1":
        raise ValueError("正文页码必须从 1 开始")
    validate_footer_relationship(files, final)


def validate_footer_relationship(files: dict[str, bytes], final: ET.Element) -> None:
    reference = final.find("w:footerReference", NS)
    if reference is None:
        raise ValueError("正文页脚引用缺失")
    rels_data = files.get("word/_rels/document.xml.rels")
    if rels_data is None:
        raise ValueError("DOCX 缺少正文关系文件")
    rels = ET.fromstring(rels_data)
    relationship_id = reference.get(qn(R, "id"))
    relationship = next((item for item in rels if item.get("Id") == relationship_id), None)
    if relationship is None or relationship.get("Type") != f"{R}/footer":
        raise ValueError("正文页脚关系无法解析")
    target = resolve_word_target(relationship.get("Target", ""))
    if not target or target not in files:
        raise ValueError("正文页脚目标文件不存在")
    footer = ET.fromstring(files[target])
    footer_text = element_text(footer)
    if "PAGE" not in footer_text or "—" not in footer_text:
        raise ValueError("正文实际页脚缺少 PAGE 字段或中文页码格式")


def resolve_word_target(target: str) -> str:
    if not target:
        return ""
    resolved = posixpath.normpath(target.lstrip("/") if target.startswith("/") else posixpath.join("word", target))
    return resolved if resolved.startswith("word/") else ""


def png_properties(data: bytes) -> tuple[int, int, int, int, int]:
    if len(data) < 33 or not data.startswith(b"\x89PNG\r\n\x1a\n"):
        raise ValueError("DOCX 包含无效 PNG")
    width, height = struct.unpack(">II", data[16:24])
    x_ppm = y_ppm = unit = 0
    offset = 8
    while offset + 12 <= len(data):
        length = struct.unpack(">I", data[offset : offset + 4])[0]
        kind = data[offset + 4 : offset + 8]
        if kind == b"pHYs" and length == 9:
            x_ppm, y_ppm, unit = struct.unpack(">IIB", data[offset + 8 : offset + 17])
            break
        offset += length + 12
    return width, height, x_ppm, y_ppm, unit


def validate_media(files: dict[str, bytes], document: ET.Element) -> None:
    media = {name: data for name, data in files.items() if name.startswith("word/media/") and name.lower().endswith(".png")}
    if len(media) != 4:
        raise ValueError("必须恰好嵌入 4 张 PNG 技术路线图")
    validate_image_relationships(files, document, set(media))
    for data in media.values():
        width, height, x_ppm, y_ppm, unit = png_properties(data)
        if width != 2400 or height <= 0 or x_ppm != 11811 or y_ppm != 11811 or unit != 1:
            raise ValueError("技术路线图必须为 2400 px 宽、300 DPI 的非空 PNG")


def validate_image_relationships(files: dict[str, bytes], document: ET.Element, media: set[str]) -> None:
    rels_data = files.get("word/_rels/document.xml.rels")
    if rels_data is None:
        raise ValueError("DOCX 缺少正文关系文件")
    relationships = {item.get("Id"): item for item in ET.fromstring(rels_data)}
    blips = document.findall(f".//{{{A}}}blip")
    relationship_ids = [item.get(qn(R, "embed"), "") for item in blips]
    if len(relationship_ids) != 4 or len(set(relationship_ids)) != 4:
        raise ValueError("正文必须通过四个独立关系嵌入技术路线图")
    targets = set()
    for blip, relationship_id in zip(blips, relationship_ids):
        relationship = relationships.get(relationship_id)
        if blip.get(qn(R, "link")) or relationship is None:
            raise ValueError("技术路线图存在外链或缺失关系")
        if relationship.get("Type") != f"{R}/image" or relationship.get("TargetMode") == "External":
            raise ValueError("技术路线图必须使用内部图片关系")
        target = resolve_word_target(relationship.get("Target", ""))
        if target not in media:
            raise ValueError("技术路线图关系未指向已验证 PNG")
        targets.add(target)
    if targets != media:
        raise ValueError("DOCX 包含孤立或重复引用的技术路线图 PNG")


def validate_contract(files: dict[str, bytes], document: ET.Element, sections: list[ET.Element]) -> None:
    validate_cover_and_headings(document)
    validate_table_captions(document)
    validate_figure_captions(document)
    validate_sections(files, sections)
    validate_media(files, document)
