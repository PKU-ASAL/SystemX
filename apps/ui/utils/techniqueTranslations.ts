/**
 * MITRE ATT&CK Technique ID 到中文名称的映射表
 * 基于 MITRE ATT&CK 框架的官方技术定义
 */

export interface TechniqueInfo {
  id?: string;
  name_en: string;
  name_cn: string;
  description?: string;
}

// 导入完整的MITRE技术翻译数据
import mitreTranslationsJson from '../mitre-techniques-with-chinese.json';

// 将导入的JSON转换为我们的接口格式
export const TECHNIQUE_TRANSLATIONS: Record<string, TechniqueInfo> = Object.fromEntries(
  Object.entries(mitreTranslationsJson).map(([id, data]) => [
    id,
    {
      id,
      name_en: (data as any).name_en,
      name_cn: (data as any).name_cn,
      description: (data as any).description
    }
  ])
);

/**
 * 获取technique的中文翻译
 * @param techniqueId - Technique ID (如 "T1566", "T1566.001")
 * @returns 中文名称，如果没有找到则返回原ID
 */
export function getTechniqueChinese(techniqueId: string): string {
  if (!techniqueId) return '';
  
  // 清理technique ID（移除引号和多余空格）
  const cleanId = techniqueId.replace(/["\s]+/g, '').trim();
  
  // 直接使用JSON翻译文件
  const technique = TECHNIQUE_TRANSLATIONS[cleanId];
  if (technique && technique.name_cn && technique.name_cn !== technique.name_en) {
    return technique.name_cn;
  }
  
  // 如果没有找到完全匹配，尝试匹配主technique（去掉子技术编号）
  const mainTechniqueId = cleanId.split('.')[0];
  const mainTechnique = TECHNIQUE_TRANSLATIONS[mainTechniqueId];
  if (mainTechnique && mainTechnique.name_cn && mainTechnique.name_cn !== mainTechnique.name_en) {
    return mainTechnique.name_cn;
  }
  
  // 如果都没找到，返回原ID
  return cleanId;
}

/**
 * 获取technique的完整信息
 * @param techniqueId - Technique ID
 * @returns TechniqueInfo对象或null
 */
export function getTechniqueInfo(techniqueId: string): TechniqueInfo | null {
  if (!techniqueId) return null;
  
  const cleanId = techniqueId.replace(/["\s]+/g, '').trim();
  return TECHNIQUE_TRANSLATIONS[cleanId] || null;
}

/**
 * 格式化technique显示文本（包含ID和中文名称）
 * @param techniqueId - Technique ID
 * @returns 格式化的显示文本
 */
export function formatTechniqueDisplay(techniqueId: string): string {
  if (!techniqueId) return '无';
  
  // 清理输入
  const cleanInput = techniqueId.replace(/["\s]+/g, '').trim();
  
  // 使用正则表达式匹配所有 T1xxx.xxx 格式的technique ID
  const techniquePattern = /T\d+(?:\.\d+)?/g;
  const techniqueIds = cleanInput.match(techniquePattern) || [];
  
  if (techniqueIds.length === 0) {
    return cleanInput;
  }
  
  if (techniqueIds.length === 1) {
    // 单个technique ID
    const singleId = techniqueIds[0];
    const chineseName = getTechniqueChinese(singleId);
    
    if (chineseName === singleId) {
      // 没有找到翻译，只返回ID
      return singleId;
    } else {
      // 返回"ID - 中文名称"的格式
      return `${singleId} - ${chineseName}`;
    }
  } else {
    // 多个technique ID，翻译所有的
    const translations = techniqueIds.map(id => {
      const chineseName = getTechniqueChinese(id);
      return chineseName === id ? id : `${id}-${chineseName}`;
    });
    // 使用换行分割，让每个技术占一行或用分号分割
    return translations.join('; ');
  }
}