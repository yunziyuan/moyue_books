import { API_BASE } from '@/config'
import { getToken } from '@/api/auth'
import { nativeFetch } from '@/utils/nativeHttp'
import { bookApi } from '@/api/book'

const SEQ_KEY = 'moyue_progress_seq_'

// 单调递增的本地序号：后端据此拒绝乱序到达的旧进度，
// 同时仍允许读者主动回退到前面的章节（新序号会被正常接受）。
function nextSeq(bookId) {
  const key = `${SEQ_KEY}${bookId}`
  let last = 0
  try { last = Number(localStorage.getItem(key)) || 0 } catch {}
  const seq = Math.max(Date.now(), last + 1)
  try { localStorage.setItem(key, String(seq)) } catch {}
  return seq
}

function sendKeepalive(bookId, payload, seq) {
  try {
    const headers = { 'Content-Type': 'application/json' }
    const token = getToken()
    if (token) headers.Authorization = `Bearer ${token}`
    nativeFetch(`${API_BASE}/books/${bookId}/progress`, {
      method: 'PUT',
      headers,
      body: JSON.stringify({
        progress: payload.progress,
        current_page: payload.currentPage,
        tts_location: payload.ttsLocation || '',
        saved_at: seq
      }),
      keepalive: true
    }).catch(() => {})
  } catch {}
}

// 每一本书独立一个写入器：请求严格串行，最后一次调用一定最后发出；
// 页面隐藏时可直接 keepalive 发送最新值，乱序旧请求由后端 saved_at 挡掉。
export function createProgressWriter(bookId) {
  let tail = Promise.resolve()

  function enqueue(payload) {
    const seq = nextSeq(bookId)
    tail = tail
      .then(() => bookApi.updateProgress(
        bookId,
        payload.progress,
        payload.currentPage,
        payload.ttsLocation || '',
        seq
      ))
      .catch(() => {})
    return tail
  }

  return {
    queue(payload) { return enqueue(payload) },
    flush() { return tail },
    flushKeepalive(payload) { sendKeepalive(bookId, payload, nextSeq(bookId)) }
  }
}
