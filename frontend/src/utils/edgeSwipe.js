// 安卓端边缘滑动导航：左边缘右滑 = 回退，右边缘左滑 = 前进。
//
// 说明：Capacitor 的 BridgeActivity 不接管返回键，系统返回键 / 系统边缘返回手势会直接
// 结束 Activity（退出 App），也没有「前进」能力。原生侧已用 OnBackPressedCallback 把系统
// 返回改成应用内回退（见 MainActivity.java），这里补充可自定义的手势，含前进。
//
// 用 history.back()/forward() 而非直接操作 router：项目是 hash 路由，Vue Router 会监听
// popstate/hashchange 自动同步视图，且能保留完整的 forward 历史项。
import { isAndroidNative } from '@/utils/platform'

// viewport 为 width=device-width，CSS px 与 Android dp 近似 1:1
const EDGE_WIDTH = 72        // 距左右边缘的有效起始区域
const MIN_DISTANCE = 56      // 触发所需的最小水平位移
const MAX_DURATION = 1200    // 慢于此时长不触发（ms）
const DIRECTION_RATIO = 1.3  // 水平位移需明显大于垂直位移，避免误伤上下滚动

export function installEdgeSwipe() {
  if (!isAndroidNative || typeof document === 'undefined') return

  let startX = 0
  let startY = 0
  let startTime = 0
  let side = ''        // 'left' | 'right'
  let swiping = false

  function reset() {
    side = ''
    swiping = false
  }

  function onTouchStart(e) {
    reset()
    if (e.touches.length !== 1) return
    const t = e.touches[0]
    startX = t.clientX
    startY = t.clientY
    startTime = Date.now()
    const width = window.innerWidth
    if (startX <= EDGE_WIDTH) side = 'left'
    else if (startX >= width - EDGE_WIDTH) side = 'right'
  }

  function onTouchMove(e) {
    if (!side || e.touches.length !== 1) return
    const dx = e.touches[0].clientX - startX
    const dy = e.touches[0].clientY - startY
    // 方向不符或更像纵向滚动：放弃本次手势，把事件交还给页面
    if (Math.abs(dx) < 8 || Math.abs(dx) < Math.abs(dy) * DIRECTION_RATIO) return
    const expected = side === 'left' ? dx > 0 : dx < 0
    if (!expected) {
      reset()
      return
    }
    swiping = true
    // 捕获阶段阻断：避免阅读器翻页 / 页面滚动与导航手势同时发生
    if (e.cancelable) e.preventDefault()
    e.stopPropagation()
  }

  function onTouchEnd(e) {
    if (!side || !swiping) {
      reset()
      return
    }
    const t = e.changedTouches && e.changedTouches[0]
    const dx = t ? t.clientX - startX : 0
    const quickEnough = Date.now() - startTime <= MAX_DURATION
    const inDirection = side === 'left' ? dx > 0 : dx < 0
    const fromSide = side
    reset()
    if (quickEnough && inDirection && Math.abs(dx) >= MIN_DISTANCE) {
      if (fromSide === 'left') window.history.back()
      else window.history.forward()
    }
  }

  const opts = { capture: true, passive: true }
  document.addEventListener('touchstart', onTouchStart, opts)
  // touchmove 需要 preventDefault，必须 non-passive
  document.addEventListener('touchmove', onTouchMove, { capture: true, passive: false })
  document.addEventListener('touchend', onTouchEnd, opts)
  document.addEventListener('touchcancel', reset, opts)
}
