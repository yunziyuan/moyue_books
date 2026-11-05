import { isAndroidNative } from '@/utils/platform'
import { nativeRequest } from '@/utils/nativeHttp'

// 安卓原生端图片加载。
// WebView 页面来源是 Capacitor 的 https://localhost，而封面 / EPUB 插图是用户填写的
// http://私有IP:端口/... 。这类跨源明文子资源会被混合内容自动升级、私有网络访问(PNA)
// 或 CORS 策略拦截，拦截发生在请求进入 WebView 网络栈之前，ProxyWebViewClient 未必能接管。
// 因此统一改由已验证可用的 HttpPlugin(原生 HttpURLConnection)取回图片二进制，转成 data:
// URL 再交给 <img>，完全不经过 WebView 网络栈。网页端不做任何处理。

const MAX_CONCURRENCY = 4
const MAX_CACHE = 200
// 1x1 透明 GIF：同步占位，避免 WebView 先发起必然失败的 http 请求并触发 @error 隐藏封面
const PLACEHOLDER = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'

const cache = new Map()
const pending = new WeakMap()
const resolved = new WeakMap()
const bypass = new WeakSet()
const queue = []
let active = 0
let seq = 0
let setRaw = (img, value) => img.setAttribute('src', value)

function enqueue(task) {
  return new Promise((resolve, reject) => {
    queue.push({ task, resolve, reject })
    pump()
  })
}

function pump() {
  while (active < MAX_CONCURRENCY && queue.length) {
    const { task, resolve, reject } = queue.shift()
    active++
    Promise.resolve().then(task).then(
      (value) => { active--; pump(); resolve(value) },
      (err) => { active--; pump(); reject(err) }
    )
  }
}

// data: URL 的 MIME 必须准确：后端封面统一存成 .jpg，内容却可能是 PNG/SVG/WebP
function sniffMime(b64) {
  if (b64.indexOf('/9j/') === 0) return 'image/jpeg'
  if (b64.indexOf('iVBOR') === 0) return 'image/png'
  if (b64.indexOf('R0lGOD') === 0) return 'image/gif'
  if (b64.indexOf('UklGR') === 0) return 'image/webp'
  if (b64.indexOf('PHN2Zy') === 0 || b64.indexOf('PD94bW') === 0) return 'image/svg+xml'
  return ''
}

function fetchDataUrl(url) {
  return enqueue(async () => {
    const res = await nativeRequest({
      method: 'GET',
      url,
      headers: {},
      data: null,
      responseType: 'blob',
      timeout: 20000
    })
    const b64 = res && res.data
    if (!res || res.status < 200 || res.status >= 300 || typeof b64 !== 'string' || !b64) {
      throw new Error(`图片请求失败 ${res ? res.status : 'unknown'}`)
    }
    const headers = res.headers || {}
    const ctKey = Object.keys(headers).find((k) => k.toLowerCase() === 'content-type')
    let mime = ctKey ? String(headers[ctKey]).split(';')[0].trim().toLowerCase() : ''
    if (mime.indexOf('image/') !== 0) mime = ''
    mime = sniffMime(b64) || mime || 'image/jpeg'
    return `data:${mime};base64,${b64}`
  })
}

function getDataUrl(url) {
  let task = cache.get(url)
  if (!task) {
    task = fetchDataUrl(url).catch((err) => {
      cache.delete(url)
      throw err
    })
    cache.set(url, task)
    if (cache.size > MAX_CACHE) cache.delete(cache.keys().next().value)
  }
  return task
}

function needsProxy(value) {
  if (typeof value !== 'string' || !/^https?:\/\//i.test(value)) return false
  try {
    return new URL(value).origin !== window.location.origin
  } catch {
    return false
  }
}

function apply(img, url) {
  const running = pending.get(img)
  if ((running && running.url === url) || resolved.get(img) === url) return
  const token = ++seq
  pending.set(img, { url, token })
  setRaw(img, PLACEHOLDER)
  getDataUrl(url).then(
    (dataUrl) => {
      const cur = pending.get(img)
      if (!cur || cur.token !== token) return
      pending.delete(img)
      resolved.set(img, url)
      // 封面组件在 @error 中会把 img 设为 visibility:hidden，取回真图后需恢复
      img.style.removeProperty('visibility')
      setRaw(img, dataUrl)
    },
    () => {
      const cur = pending.get(img)
      if (!cur || cur.token !== token) return
      pending.delete(img)
      // 原生请求失败时回退给 WebView / ProxyWebViewClient，避免完全空白
      bypass.add(img)
      setRaw(img, url)
    }
  )
}

/**
 * 封面被替换后调用：清空 data URL 缓存并让页面上已渲染的图片重新取图。
 * 非安卓原生端为空操作。
 */
export function refreshNativeImages() {
  if (!isAndroidNative || typeof document === 'undefined') return
  cache.clear()
  const imgs = document.querySelectorAll('img')
  for (let i = 0; i < imgs.length; i++) {
    const img = imgs[i]
    const running = pending.get(img)
    const url = (running && running.url) || resolved.get(img)
    if (!url) continue
    pending.delete(img)
    resolved.delete(img)
    apply(img, url)
  }
}

/**
 * 安装全局 <img> 图片代理。需在 app.mount 之前调用。
 * Vue 通过 img.src = ... 设置地址，因此先包装原型 setter 同步拦截；
 * 同时用 MutationObserver 兜底 setAttribute / innerHTML 等方式插入的图片。
 */
export function installNativeImageLoader() {
  if (!isAndroidNative || typeof HTMLImageElement === 'undefined') return

  const descriptor = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'src')
  if (descriptor && descriptor.set) {
    setRaw = (img, value) => descriptor.set.call(img, value)
  }

  function scan(img) {
    const src = img.getAttribute('src')
    if (needsProxy(src)) apply(img, src)
  }

  if (descriptor && descriptor.set && descriptor.get && descriptor.configurable) {
    try {
      Object.defineProperty(HTMLImageElement.prototype, 'src', {
        configurable: true,
        enumerable: descriptor.enumerable,
        get() { return descriptor.get.call(this) },
        set(value) {
          if (needsProxy(value)) {
            apply(this, value)
            return
          }
          pending.delete(this)
          resolved.delete(this)
          setRaw(this, value)
        }
      })
    } catch {
      // 原型不可写时仅依赖 MutationObserver
    }
  }

  const observer = new MutationObserver((records) => {
    for (const record of records) {
      if (record.type === 'attributes') {
        const img = record.target
        if (img.tagName !== 'IMG') continue
        if (bypass.has(img)) { bypass.delete(img); continue }
        scan(img)
      } else {
        const nodes = record.addedNodes
        for (let i = 0; i < nodes.length; i++) {
          const node = nodes[i]
          if (node.nodeType !== 1) continue
          if (node.tagName === 'IMG') scan(node)
          else if (node.querySelectorAll) {
            const imgs = node.querySelectorAll('img')
            for (let j = 0; j < imgs.length; j++) scan(imgs[j])
          }
        }
      }
    }
  })
  observer.observe(document.documentElement, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['src']
  })

  const initial = document.querySelectorAll('img')
  for (let i = 0; i < initial.length; i++) scan(initial[i])
}
