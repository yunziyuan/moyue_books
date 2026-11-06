// 阅读偏好本地持久化工具（设置页与阅读器共享同一份配置）
const KEY = 'reader_prefs_v1'

// 书架使用习惯：all = 显示全部书籍（默认、原有行为）；selected = 仅显示指定到书架的书籍
export const SHELF_MODE_ALL = 'all'
export const SHELF_MODE_SELECTED = 'selected'
const SHELF_MODE_EVENT = 'moyue:shelf-mode-change'

export const DEFAULT_PREFS = {
  fontSize: 16,
  lineHeight: 1.8,
  pageMargin: 40,
  fontFamily: 'system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif',
  theme: 'light',
  pdfTwoPage: false,
  pdfScale: 1.5,
  epubTwoPage: false,
  shelfMode: SHELF_MODE_ALL
}

export function loadReaderPrefs() {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULT_PREFS }
    return { ...DEFAULT_PREFS, ...JSON.parse(raw) }
  } catch {
    return { ...DEFAULT_PREFS }
  }
}

export function saveReaderPrefs(prefs) {
  try {
    // 合并写入：部分阅读器只传入单个偏好字段，整体覆盖会丢失书架习惯等其它字段
    const merged = { ...loadReaderPrefs(), ...prefs }
    localStorage.setItem(KEY, JSON.stringify(merged))
  } catch {
    /* 忽略存储异常 */
  }
}

export function resetReaderPrefs() {
  try {
    localStorage.removeItem(KEY)
  } catch {
    /* 忽略存储异常 */
  }
  return { ...DEFAULT_PREFS }
}

// ===== 书架使用习惯 =====
const normalizeShelfMode = (mode) =>
  mode === SHELF_MODE_SELECTED ? SHELF_MODE_SELECTED : SHELF_MODE_ALL

export function loadShelfMode() {
  return normalizeShelfMode(loadReaderPrefs().shelfMode)
}

export function saveShelfMode(mode) {
  const value = normalizeShelfMode(mode)
  // 合并当前存储再写回，避免覆盖阅读器 / TTS 动态写入的其它偏好字段
  const prefs = loadReaderPrefs()
  prefs.shelfMode = value
  saveReaderPrefs(prefs)
  try {
    window.dispatchEvent(new CustomEvent(SHELF_MODE_EVENT, { detail: value }))
  } catch {
    /* 忽略事件派发异常 */
  }
  return value
}

// 订阅变化：同页面自定义事件 + 跨标签页 storage 事件，返回取消订阅函数
export function onShelfModeChange(handler) {
  const onCustom = (e) => handler(normalizeShelfMode(e.detail))
  const onStorage = (e) => {
    if (e.key && e.key !== KEY) return
    handler(loadShelfMode())
  }
  window.addEventListener(SHELF_MODE_EVENT, onCustom)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(SHELF_MODE_EVENT, onCustom)
    window.removeEventListener('storage', onStorage)
  }
}
