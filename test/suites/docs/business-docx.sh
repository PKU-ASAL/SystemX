#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

OUTPUT="$TMP/sysarmor-project-proposal.zh-CN.docx"

make -C "$ROOT" business-docx BUSINESS_DOCX_OUTPUT="$OUTPUT"

[[ -s "$OUTPUT" ]] || {
  echo "[business-docx-test][ERROR] 未生成非空 DOCX：$OUTPUT" >&2
  exit 1
}
[[ "$(stat -c '%a' "$OUTPUT")" == '644' ]] || {
  echo "[business-docx-test][ERROR] 正式 DOCX 权限应为 0644，便于受控流转。" >&2
  exit 1
}

unzip -t "$OUTPUT" >/dev/null
bash "$ROOT/tools/docs/validate-business-docx.sh" "$OUTPUT"
python3 "$ROOT/test/suites/docs/business-docx-negative.py" "$OUTPUT"

content_types="$(unzip -p "$OUTPUT" '[[]Content_Types].xml')"
relationships="$(unzip -p "$OUTPUT" word/_rels/document.xml.rels)"
toc_text="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'string(//*[local-name()="sdt"])' -)"
cover_text="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'concat(string(//*[local-name()="body"]/*[local-name()="p"][1]), "|", string(//*[local-name()="body"]/*[local-name()="p"][2]), "|", string(//*[local-name()="body"]/*[local-name()="p"][3]), "|", string(//*[local-name()="body"]/*[local-name()="p"][4]))' -)"
chapter_style="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'string((//*[local-name()="p"][contains(string(.), "一、项目摘要")])[last()]/*[local-name()="pPr"]/*[local-name()="pStyle"]/@*[local-name()="val"])' -)"
principle_style="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'string((//*[local-name()="p"][contains(string(.), "动态博弈：使防御能力能够受控调整")])[last()]/*[local-name()="pPr"]/*[local-name()="pStyle"]/@*[local-name()="val"])' -)"
table_caption_count="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'count(//*[local-name()="p"][*[local-name()="pPr"]/*[local-name()="pStyle"][@*[local-name()="val"]="TableCaption"]])' -)"
figure_line_rule="$(unzip -p "$OUTPUT" word/styles.xml | xmllint --xpath 'string(//*[local-name()="style"][@*[local-name()="styleId"]="CaptionedFigure"]/*[local-name()="pPr"]/*[local-name()="spacing"]/@*[local-name()="lineRule"])' -)"
heading_color="$(unzip -p "$OUTPUT" word/styles.xml | xmllint --xpath 'string(//*[local-name()="style"][@*[local-name()="styleId"]="Heading1"]/*[local-name()="rPr"]/*[local-name()="color"]/@*[local-name()="val"])' -)"
caption_italic="$(unzip -p "$OUTPUT" word/styles.xml | xmllint --xpath 'string(//*[local-name()="style"][@*[local-name()="styleId"]="TableCaption"]/*[local-name()="rPr"]/*[local-name()="i"]/@*[local-name()="val"])' -)"
overview_break_before_heading="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'count(//*[local-name()="p"][contains(string(.), "6.3 技术路线")]/preceding-sibling::*[1]//*[local-name()="br"][@*[local-name()="type"]="page"])' -)"
overview_height="$(unzip -p "$OUTPUT" word/document.xml | xmllint --xpath 'string(//*[local-name()="docPr"][contains(@descr, "图 4")]/parent::*/*[local-name()="extent"]/@cy)' -)"
[[ "$content_types" == *'<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'* ]] || {
  echo "[business-docx-test][ERROR] 内容类型清单未使用 Office 兼容的默认命名空间。" >&2
  exit 1
}
[[ "$relationships" == *'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'* ]] || {
  echo "[business-docx-test][ERROR] 关系清单未使用 Office 兼容的默认命名空间。" >&2
  exit 1
}
[[ "$toc_text" == *'一、项目摘要'* ]] || {
  echo "[business-docx-test][ERROR] 中文目录没有生成正文条目。" >&2
  exit 1
}
[[ "$cover_text" == 'SysArmor 主机安全技术研究及平台建设项目建议书|面向政府与国有企业场景|项目建议书（讨论稿）|二〇二六年七月' ]] || {
  echo "[business-docx-test][ERROR] 中性封面四行文案不符合设计规格。" >&2
  exit 1
}
[[ "$chapter_style" == 'Heading1' && "$principle_style" == 'Heading3' ]] || {
  echo "[business-docx-test][ERROR] 正文标题未在删除重复 H1 后统一提升层级。" >&2
  exit 1
}
[[ "$toc_text" == *'动态博弈：使防御能力能够受控调整'* ]] || {
  echo "[business-docx-test][ERROR] 中文目录未收录三级标题。" >&2
  exit 1
}
[[ "$table_caption_count" == '5' ]] || {
  echo "[business-docx-test][ERROR] 五张核心表未生成连续表题。" >&2
  exit 1
}
[[ "$figure_line_rule" == 'atLeast' ]] || {
  echo "[business-docx-test][ERROR] 图片段落使用固定行距，会在 Office/WPS 中裁切图片。" >&2
  exit 1
}
[[ "$heading_color" == '000000' && ( "$caption_italic" == '0' || "$caption_italic" == 'false' ) ]] || {
  echo "[business-docx-test][ERROR] 公文标题必须为黑色，图表题注不得使用斜体。" >&2
  exit 1
}
[[ "$overview_break_before_heading" == '1' ]] || {
  echo "[business-docx-test][ERROR] 总体技术路线标题未与单页图表一起分页。" >&2
  exit 1
}
(( overview_height <= 6500000 )) || {
  echo "[business-docx-test][ERROR] 总体技术路线图过高，标题和图题无法同页。" >&2
  exit 1
}

echo "[business-docx-test] ok"
