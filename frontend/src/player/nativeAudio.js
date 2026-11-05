// 安卓原生音频后端。
//
// 背景：WebView 的 <audio src="http://服务器内网地址/..."> 由浏览器媒体栈直接发请求，
// 既拿不到 axios 的 Authorization 头，也享受不到 HttpPlugin / ProxyWebViewClient 的原生代理，
// 在 Capacitor 的 https://localhost 页面里会被 PNA / 混合内容策略拦截，导致有声书播不出声。
// 且 Activity 退到后台后 WebView 音频会被系统挂起。
//
// 方案：安卓端改用原生 MediaPlayer + 前台服务（见 android/.../AudioPlayerService.java）。
// 这里做一个「类 HTMLAudioElement」适配层，让 player/index.js 的既有逻辑完全不用改结构：
// 属性 / 方法 / 事件都保持 DOM 语义，内部转发给原生插件。
import { registerPlugin } from '@capacitor/core'
import { isAndroidNative } from '@/utils/platform'

export const isNativeAudio = isAndroidNative

const AudioPlugin = isNativeAudio ? registerPlugin('AudioPlugin') : null

// 原生事件 -> DOM 媒体事件
const EVENT_MAP = {
  ready: 'loadedmetadata',
  play: 'play',
  pause: 'pause',
  time: 'timeupdate',
  ended: 'ended',
  error: 'error',
  next: 'remoteNext',
  prev: 'remotePrev',
  toggle: 'remoteToggle',
}

export function createAudioElement() {
  if (!isNativeAudio) return new Audio()

  const listeners = new Map()
  let src = ''
  let currentTime = 0
  let duration = 0
  let playing = false
  let loading = false
  let pluginErrorReported = false

  const api = {}

  function dispatch(type, detail) {
    const fns = listeners.get(type)
    if (!fns || !fns.length) return
    const event = { type, target: api, detail }
    fns.slice().forEach((fn) => {
      try { fn(event) } catch (e) { console.warn('[nativeAudio] listener error', e) }
    })
  }

  function reportPluginError(err) {
    if (pluginErrorReported) return
    pluginErrorReported = true
    dispatch('error', {
      message: '原生播放器不可用，请重新安装最新 APK：' + ((err && err.message) || String(err)),
    })
  }

  function invoke(method, args) {
    try {
      return Promise.resolve(AudioPlugin[method](args || {})).catch(reportPluginError)
    } catch (err) {
      reportPluginError(err)
      return Promise.resolve()
    }
  }

  function onEvent(data) {
    const d = data || {}
    if (typeof d.currentTime === 'number') currentTime = d.currentTime
    if (typeof d.duration === 'number' && d.duration >= 0) duration = d.duration

    switch (d.type) {
      case 'load':
        loading = true; playing = false; currentTime = 0; duration = 0
        break
      case 'ready':
        loading = false
        break
      case 'play':
        playing = true; loading = false
        break
      case 'pause':
      case 'ended':
      case 'error':
        playing = false
        break
      case 'stop':
        playing = false; loading = false; src = ''; currentTime = 0; duration = 0
        break
      default:
        break
    }

    const type = EVENT_MAP[d.type]
    if (type) dispatch(type, d)
  }

  // Activity 重建或长时间后台后，主动拉一次状态，避免事件丢失导致 UI 与声音不同步
  async function syncState() {
    try {
      const s = await AudioPlugin.getPlaybackState()
      // 正在切段时不要用过时快照覆盖状态（IPC 返回可能晚于 load/play）
      if (!s || loading) return
      const wasPlaying = playing
      currentTime = s.currentTime || 0
      duration = s.duration || 0
      playing = !!s.playing
      loading = false
      dispatch('timeupdate')
      if (playing !== wasPlaying) dispatch(playing ? 'play' : 'pause')
    } catch {}
  }

  const listener = AudioPlugin.addListener('audioEvent', onEvent)
  if (listener && typeof listener.catch === 'function') listener.catch(() => {})
  syncState()
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) syncState()
    })
  }

  Object.assign(api, {
    preload: 'auto',

    addEventListener(type, fn) {
      if (!listeners.has(type)) listeners.set(type, [])
      listeners.get(type).push(fn)
    },
    removeEventListener(type, fn) {
      const fns = listeners.get(type)
      if (!fns) return
      const i = fns.indexOf(fn)
      if (i >= 0) fns.splice(i, 1)
    },
    getAttribute(name) {
      return name === 'src' ? src : null
    },
    removeAttribute(name) {
      // 只清空标记；真正停止由 load()（无 src 时）触发，避免切段时误停服务
      if (name === 'src') src = ''
    },
    load() {
      if (!src) {
        playing = false
        invoke('stop')
      }
    },
    play() {
      playing = true
      return invoke('play')
    },
    pause() {
      playing = false
      return invoke('pause')
    },
    setMetadata({ title = '', subtitle = '' } = {}) {
      // 静默失败：onError 会再调 updateMediaSession -> setMetadata，
      // 若这里也抛 error 事件，旧 APK（未安装原生插件）会形成无限循环。
      try {
        return Promise.resolve(AudioPlugin.setMetadata({ title, subtitle })).catch(() => {})
      } catch {
        return Promise.resolve()
      }
    },
  })

  // 注意：必须用 defineProperties 定义访问器，Object.assign 会把 getter 求值成普通数据属性
  Object.defineProperties(api, {
    src: {
      get() { return src },
      set(value) {
        src = value || ''
        if (!src) return
        loading = true
        playing = false
        currentTime = 0
        duration = 0
        invoke('load', { url: src, position: 0, title: '', subtitle: '' })
      },
    },
    currentTime: {
      get() { return currentTime },
      set(value) {
        const sec = Number(value) || 0
        currentTime = sec
        // 切段时 player 会先置 0；此时 load 本身就从头开始，无需再发一次 seek
        if (loading && sec === 0) return
        invoke('seek', { seconds: sec })
      },
    },
    duration: { get() { return duration } },
    paused: { get() { return !playing } },
  })

  return api
}
