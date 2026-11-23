<template>
  <div class="gp-mask" @click.self="$emit('close')">
    <div class="gp-panel">
      <div class="gp-panel-head">
        <div class="gpp-cover" :style="{ background: coverGradient }">{{ coverText }}</div>
        <div class="gpp-meta">
          <div class="gpp-title">{{ bookTitle }}</div>
          <div class="gpp-sub">
            {{ currentSegTitle }}
            <span v-if="waitingNext" class="gp-waiting">下一段生成后自动续播</span>
            <span v-else-if="isGenerating">边生成边播放 · 已生成 {{ doneCount }}/{{ segments.length }} 段</span>
            <span v-else-if="partial" class="gp-partial">部分生成 · 已生成 {{ doneCount }}/{{ segments.length }} 段</span>
          </div>
        </div>
        <button
          class="gpp-subs-switch"
          :class="{ active: subtitleEnabled }"
          :title="subtitleEnabled ? '关闭字幕' : '开启字幕'"
          @click="$emit('toggle-subtitle')"
        >字幕</button>
        <button class="gpp-close" @click="$emit('close')">
          <el-icon :size="18"><Close /></el-icon>
        </button>
      </div>

      <div class="gpp-tip">
        <el-icon :size="14"><InfoFilled /></el-icon>点击下方分段播放 / 暂停，点击字幕行可跳转到对应位置
      </div>

      <!-- 当前句及前后句字幕，随播放位置滚动；点击任意一句跳转 -->
      <div v-if="subtitleEnabled && visibleCues.length" class="gpp-subs">
        <div
          v-for="cue in visibleCues"
          :key="cue.i"
          class="gpp-sub-line"
          :class="{ active: cue.i === subtitleIndex }"
          @click="$emit('seek-cue', cue.start)"
        >
          <span class="gpp-sub-time">{{ formatDuration(cue.start) }}</span>
          <span class="gpp-sub-text">{{ cue.text }}</span>
        </div>
      </div>

      <div class="gpp-list">
        <div
          v-for="(seg, i) in segments"
          :key="seg.index"
          class="gpp-item"
          :class="[seg.done ? 'done' : 'pending', { active: currentIndex === i }]"
          @click="$emit('play-segment', i)"
        >
          <span class="gpp-idx">{{ seg.index }}</span>
          <div class="gpp-info">
            <div class="gpp-seg-title">{{ seg.title }}</div>
            <div class="gpp-seg-text">{{ seg.text }}</div>
          </div>
          <span class="gpp-dur">{{ seg.done ? (seg.duration ? formatDuration(seg.duration) : '--:--') : (isGenerating ? '生成中' : '未生成') }}</span>
          <span class="gpp-play-ic" v-if="seg.done">
            <el-icon :size="16"><VideoPlay /></el-icon>
          </span>
          <span class="gpp-pending-ic" v-else>
            <el-icon :size="16" :class="{ 'is-loading': isGenerating }">
              <component :is="isGenerating ? 'Loading' : 'Lock'" />
            </el-icon>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

// 解耦的播放列表弹层：仅负责展示与交互，具体播放逻辑由父级（播放器/收藏夹等）注入
const props = defineProps({
  segments: { type: Array, default: () => [] },     // 分段列表 [{ index, title, text, done, duration, file }]
  currentIndex: { type: Number, default: -1 },      // 当前播放段索引
  bookTitle: { type: String, default: '' },         // 书名
  coverText: { type: String, default: '' },         // 封面字母
  coverGradient: { type: String, default: '' },     // 封面渐变背景
  isGenerating: { type: Boolean, default: false },  // 任务是否生成中
  waitingNext: { type: Boolean, default: false },   // 是否等待续播
  subtitleCues: { type: Array, default: () => [] }, // [{ text, start, end }]
  subtitleIndex: { type: Number, default: -1 },     // 当前字幕下标
  subtitleEnabled: { type: Boolean, default: true },
})

defineEmits(['play-segment', 'close', 'seek-cue', 'toggle-subtitle'])

const currentSegTitle = computed(() => {
  const seg = props.segments[props.currentIndex]
  return seg ? seg.title || '第 ' + seg.index + ' 段' : ''
})

// 面板内只展示当前句与前一句、后一句，避免字幕挤占分段列表空间
const visibleCues = computed(() => {
  const cues = props.subtitleCues
  if (!cues.length) return []
  const idx = Math.max(0, Math.min(props.subtitleIndex, cues.length - 1))
  const from = Math.max(0, idx - 1)
  return cues.slice(from, idx + 2).map((cue, i) => ({ ...cue, i: from + i }))
})

const doneCount = computed(() => props.segments.filter((s) => s.done).length)

// 部分生成（暂停/取消/失败但保留产物）：不再自动生成，仅能播已生成段
const partial = computed(() =>
  !props.isGenerating && props.segments.some((s) => !s.done)
)

const formatDuration = (sec) => {
  if (!sec || sec <= 0) return '--:--'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}
</script>

<style scoped>
.gp-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(15, 23, 42, 0.55);
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}
.gp-mask-enter-active,
.gp-mask-leave-active {
  transition: opacity 0.25s ease;
}
.gp-mask-enter-from,
.gp-mask-leave-to {
  opacity: 0;
}

.gp-panel {
  width: min(560px, 100%);
  max-height: min(84vh, 700px);
  background: #fff;
  border-radius: 20px;
  padding: 20px 20px 14px;
  display: flex;
  flex-direction: column;
  box-shadow: 0 24px 64px rgba(15, 23, 42, 0.35);
  animation: gp-pop 0.28s cubic-bezier(0.34, 1.56, 0.64, 1);
}
@keyframes gp-pop {
  from { transform: scale(0.94) translateY(14px); opacity: 0; }
  to { transform: scale(1) translateY(0); opacity: 1; }
}

.gp-panel-head {
  display: flex;
  align-items: center;
  gap: 14px;
  padding-bottom: 14px;
  border-bottom: 1px solid #f1f5f9;
}
.gpp-cover {
  flex-shrink: 0;
  width: 52px;
  height: 52px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
  font-weight: 700;
}
.gpp-meta {
  flex: 1;
  min-width: 0;
}
.gpp-title {
  font-size: 16px;
  font-weight: 700;
  color: #1e293b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gpp-sub {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 3px;
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gp-waiting {
  flex-shrink: 0;
  font-size: 11px;
  color: #6366f1;
  background: #eef2ff;
  border-radius: 6px;
  padding: 1px 6px;
}
.gp-partial {
  flex-shrink: 0;
  font-size: 11px;
  color: #b45309;
  background: #fef3c7;
  border-radius: 6px;
  padding: 1px 6px;
  white-space: nowrap;
}
.gpp-subs-switch {
  flex-shrink: 0;
  height: 30px;
  padding: 0 9px;
  border: none;
  border-radius: 9px;
  background: #f1f5f9;
  color: #94a3b8;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}
.gpp-subs-switch.active {
  color: var(--app-primary, #6366f1);
  background: #eef2ff;
}
.gpp-subs-switch:hover {
  background: #e0e7ff;
}
.gpp-close {
  flex-shrink: 0;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  border: none;
  background: #f1f5f9;
  color: #64748b;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}
.gpp-close:hover {
  background: #fee2e2;
  color: #ef4444;
}

.gpp-tip {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 14px;
  font-size: 12px;
  color: #94a3b8;
  background: #f8fafc;
  border: 1px dashed #e2e8f0;
  border-radius: 10px;
  padding: 8px 12px;
}

.gpp-subs {
  margin-top: 12px;
  border-radius: 12px;
  border: 1px solid #e0e7ff;
  background: #f8faff;
  padding: 4px;
}
.gpp-sub-line {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s;
}
.gpp-sub-line:hover {
  background: #eef2ff;
}
.gpp-sub-line.active {
  background: linear-gradient(135deg, color-mix(in srgb, var(--app-primary, #6366f1) 14%, #fff), #fff);
}
.gpp-sub-time {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1.6;
  color: #94a3b8;
  font-variant-numeric: tabular-nums;
}
.gpp-sub-line.active .gpp-sub-time {
  color: var(--app-primary, #6366f1);
}
.gpp-sub-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.6;
  color: #64748b;
}
.gpp-sub-line.active .gpp-sub-text {
  color: #1e293b;
  font-weight: 600;
}

.gpp-list {
  margin-top: 14px;
  overflow-y: auto;
  min-height: 120px;
  max-height: 260px;
  border-radius: 12px;
  border: 1px solid #f1f5f9;
  background: #f8fafc;
  padding: 6px;
}
.gpp-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 10px;
  cursor: pointer;
  transition: background 0.15s;
}
.gpp-item.done:hover {
  background: #eef2ff;
}
.gpp-item.pending {
  cursor: default;
  opacity: 0.55;
}
.gpp-item.active {
  background: linear-gradient(135deg, color-mix(in srgb, var(--app-primary, #6366f1) 12%, #fff), #fff);
  border: 1px solid color-mix(in srgb, var(--app-primary, #6366f1) 18%, transparent);
}
.gpp-idx {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: #e2e8f0;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}
.gpp-item.active .gpp-idx {
  background: var(--app-gradient, linear-gradient(135deg, #6366f1, #8b5cf6));
  color: #fff;
}
.gpp-info {
  flex: 1;
  min-width: 0;
}
.gpp-seg-title {
  font-size: 13px;
  font-weight: 600;
  color: #334155;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gpp-item.active .gpp-seg-title {
  color: #1e293b;
}
.gpp-seg-text {
  font-size: 12px;
  color: #94a3b8;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}
.gpp-dur {
  flex-shrink: 0;
  font-size: 12px;
  color: #94a3b8;
  font-variant-numeric: tabular-nums;
}
.gpp-play-ic {
  flex-shrink: 0;
  color: var(--app-primary, #6366f1);
}
.gpp-pending-ic {
  flex-shrink: 0;
  color: #cbd5e1;
  display: flex;
}

/* ===== 移动端适配 ===== */
@media (max-width: 640px) {
  /* 播放列表：底部全屏弹层 */
  .gp-mask {
    padding: 0;
    align-items: flex-end;
  }
  .gp-panel {
    width: 100%;
    max-height: 86vh;
    border-radius: 20px 20px 0 0;
    padding: 16px 16px calc(14px + env(safe-area-inset-bottom));
  }
  .gp-panel-head {
    gap: 12px;
    padding-bottom: 12px;
  }
  .gpp-cover {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    font-size: 20px;
  }
  .gpp-title {
    font-size: 15px;
  }
  /* 提示条在手机上信息量低，隐藏让列表区域更高 */
  .gpp-tip {
    display: none;
  }
  .gpp-subs {
    margin-top: 10px;
  }
  .gpp-sub-text {
    font-size: 12px;
  }
  .gpp-list {
    margin-top: 12px;
    min-height: 0;
    max-height: 36vh;
    flex: 1;
  }
  .gpp-item {
    padding: 7px 8px;
    gap: 8px;
  }
  .gpp-idx {
    width: 24px;
    height: 24px;
    font-size: 11px;
    border-radius: 7px;
  }
  /* 手机上隐藏分段预览文本，只留标题 + 时长，列表更紧凑 */
  .gpp-seg-text {
    display: none;
  }
  .gpp-dur {
    font-size: 11px;
  }
}
</style>
