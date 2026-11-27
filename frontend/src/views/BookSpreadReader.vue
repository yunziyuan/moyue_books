<template>
  <div ref="rootEl" class="book-spread-root" :style="{ '--page-w': pageWidth + 'px', '--page-h': pageHeight + 'px' }">
    <div class="book-pages">
      <template v-if="docUrl">
        <iframe
          ref="leftFrame"
          class="book-page"
          :src="docUrl"
          sandbox="allow-same-origin"
          scrolling="no"
          @load="onFrameLoad('left')"
        />
        <!-- 双栏之间只保留一条中线 -->
        <div class="book-spine" />
        <div
          v-show="flipping"
          ref="flipSheet"
          class="book-flip-sheet"
          :style="flipStyle"
        />
        <iframe
          ref="rightFrame"
          class="book-page"
          :class="{ hidden: page + 1 >= pageCount }"
          :src="docUrl"
          sandbox="allow-same-origin"
          scrolling="no"
          @load="onFrameLoad('right')"
        />
        <div v-if="page + 1 >= pageCount" class="book-end-page">完</div>
      </template>
    </div>
    <!-- 提示层必须盖在已挂载的 iframe 之上，否则 iframe 不加载会永远停在这一步 -->
    <div v-if="loading && !error" class="book-state">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>正在排版书页…</span>
    </div>
    <div v-else-if="error" class="book-state error">{{ error }}</div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { Loading } from '@element-plus/icons-vue'

const props = defineProps({
  sections: { type: Array, required: true },
  fontCss: { type: String, default: '' },
  initialIndex: { type: Number, default: 0 },
  initialFraction: { type: Number, default: 0 },
  annotations: { type: Array, default: () => [] },
})

const emit = defineEmits(['progress', 'ready', 'selection', 'selection-clear', 'annotation-click'])

const GAP = 0   // 双栏之间不留空隙，只用一条中线分隔
const OUTER = 16
const FLIP_MS = 460

const ANN_CSS = `
.fr-ann { cursor: pointer; border-radius: 2px; }
.fr-ann[data-style="highlight"] { background-color: rgba(255,235,59,0.45); }
.fr-ann[data-style="underline"] { text-decoration: underline; text-decoration-thickness: 2px; text-underline-offset: 2px; }
.fr-ann[data-style="wavy"] { text-decoration: underline wavy; text-decoration-thickness: 2px; text-underline-offset: 3px; }
.fr-ann[data-style="strikethrough"] { text-decoration: line-through; text-decoration-thickness: 2px; }
.fr-ann[data-has-note="1"]::after { content: '📝'; font-size: 0.8em; margin-left: 2px; vertical-align: super; opacity: 0.7; }
`

const rootEl = ref(null)
const leftFrame = ref(null)
const rightFrame = ref(null)
const flipSheet = ref(null)

const loading = ref(true)
const error = ref('')
const docUrl = ref('')
const pageWidth = ref(400)
const pageHeight = ref(600)
const pageCount = ref(1)
const page = ref(0)
const flipping = ref(false)
const flipStyle = ref({})

let loadedFrames = 0
let lastProgress = { index: props.initialIndex || 0, fraction: props.initialFraction || 0, totalFraction: 0 }
let resizeTimer = null
let resizeObserver = null
let loadGuardTimer = null
let currentUrl = ''
let pendingInitialRatio = null

// 把所有章节正文合并到同一份文档：书页按整本书连续排版，左右页自然衔接
async function buildDocument() {
  loading.value = true
  error.value = ''
  loadedFrames = 0

  try {
    const sectionStyles = []
    const bodies = await Promise.all(props.sections.map(async (sec, i) => {
      const src = await sec.load()
      const html = await fetch(src).then(r => r.text())
      const doc = new DOMParser().parseFromString(html, 'text/html')
      const styles = Array.from(doc.querySelectorAll('style'))
        .map(s => s.textContent || '')
        .filter(Boolean)
        .join('\n')
      if (styles) sectionStyles.push(styles)
      return `<div class="book-section" data-index="${i}">${doc.body?.innerHTML || ''}</div>`
    }))

    const html = `<!doctype html><html><head><meta charset="utf-8"><style>${props.fontCss}</style><style>${sectionStyles.join('\n')}</style></head><body>${bodies.join('')}</body></html>`
    if (currentUrl) URL.revokeObjectURL(currentUrl)
    currentUrl = URL.createObjectURL(new Blob([html], { type: 'text/html' }))
    docUrl.value = currentUrl
  } catch (e) {
    console.warn('[BookSpreadReader] build document failed', e)
    error.value = '书页排版失败，请重试'
    loading.value = false
  }
}

function measureGeometry() {
  const el = rootEl.value
  if (!el) return
  const usable = Math.max(400, el.clientWidth - GAP - OUTER * 2)
  pageWidth.value = Math.floor(usable / 2)
  pageHeight.value = Math.max(320, el.clientHeight - OUTER * 2)
}

function pageCss() {
  const w = pageWidth.value
  const h = pageHeight.value
  return `
    html {
      margin: 0 !important; padding: 0 !important;
      height: 100% !important; overflow: hidden !important;
    }
    body {
      margin: 0 !important;
      padding: 20px 0 !important;
      width: ${w}px !important;
      height: ${h}px !important;
      column-width: ${w}px !important;
      column-gap: 0 !important;
      column-fill: auto !important;
      overflow-x: auto !important;
      overflow-y: hidden !important;
      scrollbar-width: none !important;
    }
    body::-webkit-scrollbar { display: none !important; }
    .book-section { padding: 0 32px; box-sizing: border-box; }
    ${ANN_CSS}
    p, li, blockquote, dd, h1, h2, h3, h4, h5, h6, figure, img, svg, table {
      break-inside: avoid !important;
      page-break-inside: avoid !important;
      -webkit-column-break-inside: avoid !important;
    }
  `
}

function injectPageCss(frame) {
  const doc = frame?.contentDocument
  if (!doc?.body) return
  let style = doc.getElementById('__book_page_style')
  if (!style) {
    style = doc.createElement('style')
    style.id = '__book_page_style'
    doc.head?.appendChild(style)
  }
  style.textContent = pageCss()
  if (!doc.__bookWheelBlocked) {
    // 双栏翻页只允许点击热区翻页，屏蔽滚轮与触摸滑动
    doc.addEventListener('wheel', e => e.preventDefault(), { passive: false })
    doc.addEventListener('touchmove', e => e.preventDefault(), { passive: false })
    doc.__bookWheelBlocked = true
  }
}

function findTextRange(root, text, matchIndex = 0) {
  if (!text) return null
  const walker = root.ownerDocument.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const nodes = []
  let node, full = ''
  while ((node = walker.nextNode())) {
    nodes.push({ node, start: full.length })
    full += node.textContent
  }
  let from = 0, found = -1
  for (let i = 0; i <= matchIndex; i++) {
    found = full.indexOf(text, from)
    if (found < 0) return null
    from = found + text.length
  }
  let sNode, sOff, eNode, eOff
  for (const { node: n, start } of nodes) {
    const end = start + n.textContent.length
    if (!sNode && end > found) { sNode = n; sOff = found - start }
    if (end >= found + text.length) { eNode = n; eOff = found + text.length - start; break }
  }
  if (!sNode || !eNode) return null
  const range = root.ownerDocument.createRange()
  try {
    range.setStart(sNode, sOff)
    range.setEnd(eNode, eOff)
    return range
  } catch { return null }
}

function makeAnnSpan(doc, ann, text) {
  const span = doc.createElement('span')
  span.className = 'fr-ann'
  span.dataset.id = String(ann.id)
  span.dataset.style = ann.style || 'highlight'
  span.dataset.hasNote = ann.note ? '1' : '0'
  if (text) span.textContent = text
  return span
}

function applyAnnotationToFrame(frame, ann) {
  const doc = frame?.contentDocument
  if (!doc?.body) return
  if (doc.querySelector(`.fr-ann[data-id="${ann.id}"]`)) return
  let pos = null
  try { pos = JSON.parse(ann.position || '{}') } catch { /* noop */ }
  if (pos?.kind !== 'cr' || typeof pos.index !== 'number') return
  const section = doc.querySelector(`.book-section[data-index="${pos.index}"]`)
  if (!section) return
  const range = findTextRange(section, pos.text || ann.content || '', pos.matchIndex || 0)
  if (!range) return
  try {
    if (range.startContainer === range.endContainer && range.startContainer.nodeType === Node.TEXT_NODE) {
      const node = range.startContainer
      const frag = doc.createDocumentFragment()
      const before = node.textContent.slice(0, range.startOffset)
      const mid = node.textContent.slice(range.startOffset, range.endOffset)
      const after = node.textContent.slice(range.endOffset)
      if (before) frag.appendChild(doc.createTextNode(before))
      frag.appendChild(makeAnnSpan(doc, ann, mid))
      if (after) frag.appendChild(doc.createTextNode(after))
      node.parentNode.replaceChild(frag, node)
    } else {
      const span = makeAnnSpan(doc, ann)
      span.appendChild(range.extractContents())
      range.insertNode(span)
    }
  } catch { /* noop */ }
}

function applyAllAnnotations(frame) {
  for (const ann of props.annotations || []) applyAnnotationToFrame(frame, ann)
}

function bindDocEvents(frame) {
  const doc = frame?.contentDocument
  if (doc?.__bookEventsBound) return
  doc.__bookEventsBound = true
  const sectionIndex = node => {
    const el = node?.nodeType === Node.ELEMENT_NODE ? node : node?.parentElement
    const sec = el?.closest?.('.book-section')
    return sec ? Number(sec.dataset.index) : 0
  }
  doc.addEventListener('mouseup', () => {
    const sel = doc.getSelection?.()
    if (!sel || sel.isCollapsed || !sel.rangeCount) { emit('selection-clear'); return }
    const text = sel.toString().trim()
    if (text.length < 2) { emit('selection-clear'); return }
    const rect = sel.getRangeAt(0).getBoundingClientRect()
    const frameRect = frame.getBoundingClientRect()
    emit('selection', {
      blockIdx: sectionIndex(sel.anchorNode),
      text,
      x: frameRect.left + rect.left + rect.width / 2,
      y: frameRect.top + rect.top - 6,
      rect: { top: frameRect.top + rect.top, left: frameRect.left + rect.left, width: rect.width, height: rect.height },
    })
  })
  doc.addEventListener('mousedown', () => emit('selection-clear'))
  doc.addEventListener('click', e => {
    const ann = e.target?.closest?.('.fr-ann')
    if (ann?.dataset.id) emit('annotation-click', { id: Number(ann.dataset.id), blockIdx: 0 })
  })
}

function applyPage(frame, target) {
  const doc = frame?.contentDocument
  if (!doc?.body) return
  const max = Math.max(0, pageCount.value - 1)
  const p = Math.min(max, Math.max(0, target))
  doc.body.scrollLeft = p * pageWidth.value
}

function applyPages() {
  applyPage(leftFrame.value, page.value)
  applyPage(rightFrame.value, page.value + 1)
}

// 章节在页流中的起始页（合并文档里每章都有 .book-section 包裹，可直接量位置）
function sectionStartPages() {
  const doc = leftFrame.value?.contentDocument
  if (!doc?.body) return null
  const bodyRect = doc.body.getBoundingClientRect()
  const starts = []
  doc.querySelectorAll('.book-section').forEach(el => {
    const x = el.getBoundingClientRect().left - bodyRect.left + doc.body.scrollLeft
    starts.push(Math.max(0, Math.floor(x / pageWidth.value)))
  })
  return starts.length ? starts : null
}

function emitProgress() {
  const total = props.sections.length || 1
  const maxPage = Math.max(0, pageCount.value - 1)
  const ratio = maxPage > 0 ? page.value / maxPage : 0
  const starts = sectionStartPages()
  let index = 0
  let fraction = ratio
  if (starts) {
    for (let i = 0; i < starts.length; i++) {
      if (starts[i] <= page.value) index = i
    }
    const next = starts[index + 1] ?? (pageCount.value + 1)
    fraction = Math.max(0, Math.min(1, (page.value - starts[index]) / Math.max(1, next - starts[index])))
  } else {
    const raw = Math.min(total, ratio * total)
    index = Math.min(total - 1, Math.floor(raw))
    fraction = Math.max(0, Math.min(1, raw - index))
  }
  lastProgress = { index, fraction, totalFraction: ratio }
  emit('progress', lastProgress)
}

function measurePages(preserveRatio = false) {
  const doc = leftFrame.value?.contentDocument
  if (!doc?.body) return
  const oldRatio = pageCount.value > 1 ? page.value / (pageCount.value - 1) : 0
  requestAnimationFrame(() => requestAnimationFrame(() => {
    const sw = doc.body.scrollWidth || pageWidth.value
    pageCount.value = Math.max(1, Math.round(sw / pageWidth.value))
    const total = props.sections.length || 1
    let ratio = preserveRatio ? oldRatio : pendingInitialRatio
    if (ratio == null) {
      const starts = sectionStartPages()
      const idx = Math.min(total - 1, Math.max(0, props.initialIndex || 0))
      const start = starts?.[idx] ?? Math.round((idx / total) * (pageCount.value - 1))
      const end = starts?.[idx + 1] ?? pageCount.value
      const span = Math.max(1, end - start)
      ratio = (start + (props.initialFraction || 0) * (span - 1)) / Math.max(1, pageCount.value - 1)
    }
    ratio = Math.min(1, Math.max(0, ratio))
    pendingInitialRatio = null
    page.value = Math.min(pageCount.value - 1, Math.round(ratio * (pageCount.value - 1)))
    applyPages()
    emitProgress()
  }))
}

function onFrameLoad(side) {
  const frame = side === 'left' ? leftFrame.value : rightFrame.value
  nextTick(() => {
    injectPageCss(frame)
    applyAllAnnotations(frame)
    bindDocEvents(frame)
    applyPage(frame, side === 'left' ? page.value : page.value + 1)
    loadedFrames++
    if (loadedFrames >= 2) {
      clearTimeout(loadGuardTimer)
      // 先把恢复出的锚点同步给外壳，避免测量完成前的 0% 覆盖本地进度
      emit('progress', lastProgress)
      measurePages()
      loading.value = false
      emit('ready')
    }
  })
}

function waitFlipDone() {
  return new Promise(resolve => {
    let done = false
    const finish = () => {
      if (done) return
      done = true
      flipSheet.value?.removeEventListener('transitionend', finish)
      resolve()
    }
    flipSheet.value?.addEventListener('transitionend', finish)
    setTimeout(finish, FLIP_MS + 120)
  })
}

async function pageFlip(direction) {
  if (flipping.value || loading.value || error.value) return
  const maxPage = pageCount.value - 1
  const next = direction > 0
  const oldPage = page.value
  // 一次翻整组双栏：左栏不再重复显示刚才右栏的内容
  if (next && oldPage + 2 > maxPage) return
  if (!next && oldPage - 2 < 0) return

  const targetLeft = oldPage + (next ? 2 : -2)
  const targetRight = targetLeft + 1
  flipping.value = true

  // 翻页纸下方先备好目标页：下一组先换右栏，上一组先换左栏
  applyPage(next ? rightFrame.value : leftFrame.value, next ? targetRight : targetLeft)

  const base = next
    ? { left: '50%', transformOrigin: 'left center' }
    : { left: 'calc(50% - var(--page-w))', transformOrigin: 'right center' }
  flipStyle.value = { ...base, transform: 'rotateY(0deg)' }
  await nextTick()
  // 强制回流，保证起始变换先落地再开始动画
  void flipSheet.value?.offsetWidth
  flipStyle.value = { ...base, transform: next ? 'rotateY(-180deg)' : 'rotateY(180deg)' }

  await waitFlipDone()
  applyPage(next ? leftFrame.value : rightFrame.value, next ? targetLeft : targetRight)
  page.value = targetLeft
  flipping.value = false
  flipStyle.value = { ...base, transform: 'rotateY(0deg)' }
  applyPages()
  emitProgress()
}

function goToPage(target) {
  if (loading.value || !pageCount.value) return
  const max = pageCount.value - 1
  page.value = Math.min(max, Math.max(0, target))
  applyPages()
  emitProgress()
}
function scrollToSection(index) {
  const total = props.sections.length || 1
  const starts = sectionStartPages()
  const target = starts?.[index] ?? Math.round((index / total) * (pageCount.value - 1))
  goToPage(target)
}
function scrollToFraction(fraction) {
  goToPage(Math.round((fraction || 0) * (pageCount.value - 1)))
}

// 以下接口与 ScrollContinuousReader 对齐，供外壳的目录/进度/朗读逻辑统一调用
function getProgress() { return lastProgress }
function setCursor() {}
function clearSentenceHighlight() {}
function resetSentenceSearch() {}
function scrollHighlightedIntoView() {}
function highlightSentence() {}
async function loadSection() {}
function isSectionLoaded() { return true }
function getSectionText(index) {
  const doc = leftFrame.value?.contentDocument
  return doc?.querySelector(`.book-section[data-index="${index}"]`)?.innerText || ''
}
function getSectionTextLength(index) { return getSectionText(index).length }
function clearSelection() {
  for (const frame of [leftFrame.value, rightFrame.value]) {
    try { frame?.contentDocument?.getSelection?.()?.removeAllRanges?.() } catch { /* noop */ }
  }
}
function applyAnnotation(ann) {
  applyAnnotationToFrame(leftFrame.value, ann)
  applyAnnotationToFrame(rightFrame.value, ann)
  return true
}
function removeAnnotation(id) {
  for (const frame of [leftFrame.value, rightFrame.value]) {
    const doc = frame?.contentDocument
    doc?.querySelectorAll(`.fr-ann[data-id="${id}"]`).forEach(el => el.replaceWith(...el.childNodes))
  }
}
function scrollToAnnotation() {}

watch(() => props.fontCss, async () => {
  pendingInitialRatio = pageCount.value > 1 ? page.value / (pageCount.value - 1) : 0
  await buildDocument()
})

watch(() => props.annotations, () => {
  if (loading.value) return
  applyAllAnnotations(leftFrame.value)
  applyAllAnnotations(rightFrame.value)
}, { deep: true })

onMounted(async () => {
  measureGeometry()
  // 兜底：任何异常导致 load 事件缺失时也不能一直停在"正在排版书页"
  loadGuardTimer = setTimeout(() => {
    if (!rootEl.value || !loading.value) return
    const doc = leftFrame.value?.contentDocument
    if (doc?.body?.querySelector?.('.book-section')) {
      injectPageCss(leftFrame.value)
      injectPageCss(rightFrame.value)
      measurePages()
      loading.value = false
      emit('ready')
    } else {
      error.value = '书页加载超时，请切换版式后重试'
      loading.value = false
    }
  }, 10000)
  await buildDocument()
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      clearTimeout(resizeTimer)
      resizeTimer = setTimeout(() => {
        measureGeometry()
        injectPageCss(leftFrame.value)
        injectPageCss(rightFrame.value)
        measurePages(true)
      }, 200)
    })
    resizeObserver.observe(rootEl.value)
  }
})

onBeforeUnmount(() => {
  clearTimeout(resizeTimer)
  clearTimeout(loadGuardTimer)
  resizeObserver?.disconnect()
  if (currentUrl) URL.revokeObjectURL(currentUrl)
  for (const sec of props.sections) {
    try { sec.unload?.() } catch { /* noop */ }
  }
})

defineExpose({
  pageFlip, scrollToSection, scrollToFraction, getProgress,
  setCursor, clearSentenceHighlight, resetSentenceSearch, scrollHighlightedIntoView,
  highlightSentence, loadSection, isSectionLoaded, getSectionText, getSectionTextLength,
  clearSelection, applyAnnotation, removeAnnotation, scrollToAnnotation,
})
</script>

<style scoped>
.book-spread-root {
  width: 100%;
  height: 100%;
  position: relative;
  overflow: hidden;
  background: var(--cr-bg, #f4f4f5);
}
.book-state {
  position: absolute; inset: 0; z-index: 20;
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 10px; color: #64748b; font-size: 14px;
  background: var(--cr-bg, #f4f4f5);
}
.book-state.error { color: #ef4444; }
.book-pages {
  width: 100%; height: 100%;
  display: flex; align-items: center; justify-content: center;
  gap: 0;
  perspective: 2200px;
  position: relative;
}
.book-page {
  width: var(--page-w);
  height: var(--page-h);
  border: 0;
  background: var(--cr-paper, #fdfaf3);
  display: block;
  flex: 0 0 auto;
}
.book-page.hidden { visibility: hidden; }
/* 双栏之间只有一条中线，不再留空隙或投影 */
.book-spine {
  position: absolute;
  left: 50%;
  top: 16px;
  width: 1px;
  height: var(--page-h);
  background: color-mix(in srgb, var(--el-text-color-primary, #1f2937) 14%, transparent);
  z-index: 6;
  pointer-events: none;
}
.book-end-page {
  position: absolute;
  left: 50%;
  width: var(--page-w);
  text-align: center;
  top: 50%;
  transform: translateY(-50%);
  color: #94a3b8;
  font-size: 13px;
  letter-spacing: 4px;
  pointer-events: none;
}
.book-flip-sheet {
  position: absolute;
  top: 16px;
  width: var(--page-w);
  height: var(--page-h);
  z-index: 5;
  background: var(--cr-paper, #fdfaf3);
  background-image: repeating-linear-gradient(
    to bottom,
    transparent 0,
    transparent 27px,
    rgba(120, 120, 120, 0.06) 28px,
    transparent 29px
  );
  box-shadow: 0 2px 18px rgba(0, 0, 0, 0.22);
  transform-style: preserve-3d;
  /* 翻到 180° 时仍需用纸背遮住换页瞬间，不能隐藏背面 */
  transition: transform 0.46s cubic-bezier(0.22, 0.61, 0.36, 1);
  pointer-events: none;
}
</style>
