<template>
  <div class="settings-section releasenotes-section">
    <div class="section-title">
      <el-icon><Histogram /></el-icon>
      <div>
        <h3>版本升级说明</h3>
        <p>当前版本 v{{ APP_VERSION }}，以下为各版本的更新内容</p>
      </div>
    </div>

    <div class="rn-timeline">
      <div
        v-for="(item, idx) in releaseNotes"
        :key="item.version"
        class="rn-item"
        :class="{ current: item.version === APP_VERSION }"
      >
        <div class="rn-dot">
          <el-icon v-if="item.version === APP_VERSION"><StarFilled /></el-icon>
        </div>
        <div class="rn-card">
          <div class="rn-head">
            <span class="rn-version">v{{ item.version }}</span>
            <el-tag v-if="item.version === APP_VERSION" type="success" size="small" effect="dark">当前</el-tag>
            <span class="rn-date">{{ item.date }}</span>
          </div>
          <ul class="rn-changes">
            <li v-for="(c, i) in item.changes" :key="i">{{ c }}</li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Histogram, StarFilled } from '@element-plus/icons-vue'
import { APP_VERSION } from '@/utils/version'

// 版本升级说明（更新时请在最上方追加新版本条目，并保持与 fnos/moyue/manifest 版本一致）
const releaseNotes = [
  {
    version: '1.0.2',
    date: '2026-10',
    changes: [
      '新增「开源说明」设置，展示 MIT 协议与开源地址',
      '阅读器与有声书体验优化',
      '修复若干已知问题'
    ]
  },
  {
    version: '1.0.1',
    date: '2026-09',
    changes: [
      'Edge TTS 后端改为 Go 原生直连，规避受限网络 403 问题',
      '优化阅读进度同步与书架性能',
      '修复若干已知问题'
    ]
  },
  {
    version: '1.0.0',
    date: '2026-08',
    changes: [
      '首个正式版本发布',
      '支持 EPUB / PDF / MOBI / AZW3 / CBZ / 有声书等多种格式',
      '在线阅读器：目录导航、翻页、进度保存、阅读偏好',
      '云端 / 本地引擎语音合成（TTS）与有声书生成',
      '阅读统计、划线批注、账号与外观设置'
    ]
  }
]
</script>

<style scoped>
.section-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}
.section-title > .el-icon {
  font-size: 26px;
  color: var(--el-color-primary);
  background: color-mix(in srgb, var(--app-primary) 12%, transparent);
  padding: 10px;
  border-radius: 12px;
}
.section-title h3 {
  margin: 0;
  font-size: 18px;
}
.section-title p {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.rn-timeline {
  position: relative;
  padding-left: 8px;
}
.rn-timeline::before {
  content: '';
  position: absolute;
  left: 19px;
  top: 6px;
  bottom: 6px;
  width: 2px;
  background: var(--el-border-color-lighter);
}
.rn-item {
  position: relative;
  display: flex;
  gap: 16px;
  padding-bottom: 20px;
}
.rn-item:last-child {
  padding-bottom: 0;
}
.rn-dot {
  position: relative;
  z-index: 1;
  flex: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--el-bg-color);
  border: 2px solid var(--el-border-color);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  margin-top: 2px;
}
.rn-item.current .rn-dot {
  background: linear-gradient(135deg, var(--app-primary), color-mix(in srgb, var(--app-primary) 55%, #8b5cf6));
  border-color: transparent;
}
.rn-item.current .rn-dot .el-icon {
  font-size: 13px;
}
.rn-card {
  flex: 1;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  background: var(--el-bg-color);
  box-shadow: 0 8px 24px rgba(31, 38, 135, 0.06);
  padding: 16px 18px;
}
.rn-item.current .rn-card {
  border-color: color-mix(in srgb, var(--app-primary) 35%, transparent);
}
.rn-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.rn-version {
  font-size: 15px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}
.rn-date {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.rn-changes {
  margin: 0;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rn-changes li {
  font-size: 13px;
  color: var(--el-text-color-regular);
  line-height: 1.6;
}

@media (max-width: 768px) {
  .rn-card {
    padding: 14px 14px;
  }
}
</style>
