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

// MITRE技术翻译数据缓存
let TECHNIQUE_TRANSLATIONS: Record<string, TechniqueInfo> = {};
let isLoaded = false;

// 异步加载MITRE技术翻译数据
async function loadTechniqueTranslations(): Promise<void> {
  if (isLoaded) return;

  try {
    const response = await fetch('/data/mitre-techniques-with-chinese.json');
    if (!response.ok) {
      throw new Error(`Failed to load MITRE translations: ${response.status}`);
    }

    const mitreTranslationsJson = await response.json();

    // 将导入的JSON转换为我们的接口格式
    TECHNIQUE_TRANSLATIONS = Object.fromEntries(
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

    isLoaded = true;
  } catch (error) {
    console.error('Failed to load MITRE technique translations:', error);
    // 使用空对象作为fallback
    TECHNIQUE_TRANSLATIONS = {};
    isLoaded = true;
  }
}

// 确保数据已加载的辅助函数
async function ensureTranslationsLoaded(): Promise<void> {
  if (!isLoaded) {
    await loadTechniqueTranslations();
  }
}

/**
 * 获取technique的中文翻译（异步版本）
 * @param techniqueId - Technique ID (如 "T1566", "T1566.001")
 * @returns 中文名称，如果没有找到则返回原ID
 */
export async function getTechniqueChinese(techniqueId: string): Promise<string> {
  if (!techniqueId) return '';

  await ensureTranslationsLoaded();

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
 * 获取technique的中文翻译（同步版本，用于已加载数据的情况）
 * @param techniqueId - Technique ID (如 "T1566", "T1566.001")
 * @returns 中文名称，如果没有找到则返回原ID
 */
export function getTechniqueChineseSync(techniqueId: string): string {
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
export async function getTechniqueInfo(techniqueId: string): Promise<TechniqueInfo | null> {
  if (!techniqueId) return null;

  await ensureTranslationsLoaded();

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
    const chineseName = getTechniqueChineseSync(singleId);

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
      const chineseName = getTechniqueChineseSync(id);
      return chineseName === id ? id : `${id}-${chineseName}`;
    });
    // 使用换行分割，让每个技术占一行或用分号分割
    return translations.join('; ');
  }
}

/**
 * 初始化翻译数据（在应用启动时调用）
 */
export async function initializeTechniqueTranslations(): Promise<void> {
  await loadTechniqueTranslations();
}
