local page_break = pandoc.RawBlock(
  "openxml",
  '<w:p><w:r><w:br w:type="page"/></w:r></w:p>'
)

local table_titles = {
  "现有方案能力边界",
  "当前能力边界",
  "实施计划",
  "考核指标",
  "业务概念与工程名称对照",
}

local function metadata(text)
  return pandoc.MetaInlines({pandoc.Str(text)})
end

local function is_figure(block)
  return block.t == "Para"
    and #block.content == 1
    and block.content[1].t == "Image"
end

local function caption_text(block)
  if block == nil or block.t ~= "Para" then
    return nil
  end
  local text = pandoc.utils.stringify(block.content)
  return text:match("^图：%s*(.+)$")
end

local function png_target(target)
  return target:gsub("%.svg$", ".png")
end

local function figure_caption(number, text)
  return {pandoc.Str(string.format("图 %d　%s", number, text))}
end

local function table_caption(number, text)
  local caption = pandoc.Para({pandoc.Str(string.format("表 %d　%s", number, text))})
  local attributes = pandoc.Attr("", {}, {{"custom-style", "TableCaption"}})
  return pandoc.Div({caption}, attributes)
end

local function configure_image(image, number, text)
  image.src = png_target(image.src)
  image.caption = figure_caption(number, text)
  if image.src:find("technical%-roadmap%-nsfc%-overview") then
    image.attributes.width = "120mm"
  else
    image.attributes.width = "154mm"
  end
  return image
end

local function is_duplicate_title(block)
  return block.t == "Header"
    and block.level == 1
    and pandoc.utils.stringify(block.content):find("SysArmor 主机安全技术研究及平台建设项目建议书", 1, true)
end

local function transform_blocks(blocks)
  local output = {}
  local index = 1
  local figure_number = 0
  local table_number = 0
  local title_removed = false
  while index <= #blocks do
    local block = blocks[index]
    if not title_removed and is_duplicate_title(block) then
      title_removed = true
    elseif block.t == "Table" then
      table_number = table_number + 1
      table.insert(output, table_caption(table_number, table_titles[table_number]))
      table.insert(output, block)
    elseif is_figure(block) and caption_text(blocks[index + 1]) then
      figure_number = figure_number + 1
      local text = caption_text(blocks[index + 1])
      local image = configure_image(block.content[1], figure_number, text)
      local overview = image.src:find("technical%-roadmap%-nsfc%-overview") ~= nil
      if overview then
        local heading = output[#output]
        if heading and heading.t == "Header" then
          table.remove(output)
          table.insert(output, page_break)
          table.insert(output, heading)
        else
          table.insert(output, page_break)
        end
      end
      table.insert(output, pandoc.Para({image}))
      if overview then
        table.insert(output, page_break)
      end
      index = index + 1
    elseif block.t == "Header" then
      block.level = math.max(1, block.level - 1)
      table.insert(output, block)
    else
      table.insert(output, block)
    end
    index = index + 1
  end
  return output
end

function Pandoc(document)
  document.meta.title = metadata("SysArmor 主机安全技术研究及平台建设项目建议书")
  document.meta.subtitle = metadata("面向政府与国有企业场景")
  document.meta.author = metadata("项目建议书（讨论稿）")
  document.meta.date = metadata("二〇二六年七月")
  document.meta["toc-title"] = metadata("目　录")
  return pandoc.Pandoc(transform_blocks(document.blocks), document.meta)
end
