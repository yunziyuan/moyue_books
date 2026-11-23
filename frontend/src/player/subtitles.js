// 有声书字幕：把分段全文切成短句，并按实际音频时长分配每句时间轴。
// 后端 segments.json 已保存每段完整文本（此前仅用于 TTS 合成），无需生成 sidecar
// 字幕文件或重新合成音频；标点/段落作为停顿权重，使时间轴贴近朗读节奏。

const MAX_CUE_CHARS = 60 // 单条字幕最大字数，超长句在最近的逗号/顿号处断开
const SENTENCE_END_RE = /[。！？!?；;…]+/
const SOFT_BREAK_CHARS = '，,、：:'

// 按段落与句读切分朗读文本，返回可直接显示的字幕行
export function splitSubtitleLines(text) {
  const lines = []
  for (const para of String(text || '').split(/\n+/)) {
    const p = para.trim()
    if (!p) continue
    let buf = ''
    const flush = () => {
      const t = buf.trim()
      if (t) lines.push(t)
      buf = ''
    }
    for (const ch of p) {
      buf += ch
      if (SENTENCE_END_RE.test(ch)) {
        flush()
        continue
      }
      if ([...buf].length >= MAX_CUE_CHARS) {
        // 优先在最后一个软停顿处断开，避免把词组从中间截断
        let cut = -1
        for (let i = buf.length - 1; i >= Math.floor(buf.length / 2); i--) {
          if (SOFT_BREAK_CHARS.includes(buf[i])) { cut = i; break }
        }
        if (cut >= 0) {
          lines.push(buf.slice(0, cut + 1).trim())
          buf = buf.slice(cut + 1)
        } else {
          flush()
        }
      }
    }
    flush()
  }
  return lines
}

// 停顿权重：句末标点稍长，逗号次之，空白不计时；保证句子越长时间越长
function lineWeight(line) {
  let w = 0
  for (const ch of line) {
    if (/\s/.test(ch)) continue
    if (SOFT_BREAK_CHARS.includes(ch)) w += 0.4
    else if (SENTENCE_END_RE.test(ch)) w += 0.8
    else w += 1
  }
  return Math.max(1, w)
}

// 生成 [{ text, start, end }]；duration 为当前段实际音频时长（秒）
export function buildSubtitleCues(text, duration) {
  const lines = splitSubtitleLines(text)
  if (!lines.length) return []
  const total = Number(duration) > 0 ? Number(duration) : lines.length
  const weights = lines.map(lineWeight)
  const sum = weights.reduce((a, b) => a + b, 0)
  let acc = 0
  return lines.map((line, i) => {
    const start = (acc / sum) * total
    acc += weights[i]
    return { text: line, start, end: (acc / sum) * total }
  })
}

// 当前时间命中的字幕下标（二分查找，拖动进度条后同样适用）
export function findCueIndex(cues, time) {
  if (!cues || !cues.length) return -1
  const t = Number(time) || 0
  let lo = 0
  let hi = cues.length - 1
  let found = 0
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (cues[mid].start <= t) { found = mid; lo = mid + 1 }
    else hi = mid - 1
  }
  return found
}
