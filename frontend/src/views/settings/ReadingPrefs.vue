<template>
  <div class="settings-section reading-prefs">
    <div class="section-title">
      <el-icon><Reading /></el-icon>
      <div>
        <h3>阅读偏好</h3>
        <p>调整阅读器的主题、字体与排版，修改后立即生效并自动保存</p>
      </div>
    </div>

    <!-- 主题 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>阅读主题</span>
      </div>
      <div class="theme-cards">
        <div
          v-for="t in themeList"
          :key="t.key"
          class="theme-card"
          :class="{ active: prefs.theme === t.key }"
          @click="setPref('theme', t.key)"
        >
          <div class="theme-swatch" :style="t.swatchStyle">
            <div class="theme-line" :style="{ background: t.lineColor }"></div>
            <div class="theme-line short" :style="{ background: t.lineColor }"></div>
            <div class="theme-line" :style="{ background: t.lineColor }"></div>
          </div>
          <div class="theme-meta">
            <span class="theme-name">{{ t.label }}</span>
            <el-icon v-if="prefs.theme === t.key" class="theme-check"><Check /></el-icon>
          </div>
        </div>
      </div>
    </div>

    <!-- 字体大小 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>字体大小</span>
        <span class="pref-value">{{ prefs.fontSize }}px</span>
      </div>
      <el-slider
        :model-value="prefs.fontSize"
        :min="12"
        :max="28"
        :step="1"
        show-stops
        @update:model-value="setPref('fontSize', $event)"
      />
    </div>

    <!-- 行间距 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>行间距</span>
        <span class="pref-value">{{ prefs.lineHeight.toFixed(1) }}</span>
      </div>
      <el-slider
        :model-value="prefs.lineHeight"
        :min="1.4"
        :max="2.6"
        :step="0.1"
        @update:model-value="setPref('lineHeight', $event)"
      />
    </div>

    <!-- 页面边距 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>页面边距</span>
        <span class="pref-value">{{ prefs.pageMargin }}px</span>
      </div>
      <el-slider
        :model-value="prefs.pageMargin"
        :min="0"
        :max="80"
        :step="2"
        show-stops
        @update:model-value="setPref('pageMargin', $event)"
      />
    </div>

    <!-- 字体 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>正文字体</span>
      </div>
      <el-select
        :model-value="prefs.fontFamily"
        style="width: 100%"
        @update:model-value="setPref('fontFamily', $event)"
      >
        <el-option
          v-for="f in fontOptions"
          :key="f.value"
          :label="f.label"
          :value="f.value"
        >
          <span :style="{ fontFamily: f.value }">{{ f.label }}</span>
        </el-option>
      </el-select>
    </div>

    <!-- 书架使用习惯 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>书架使用习惯</span>
      </div>
      <div class="shelf-cards">
        <div
          v-for="m in shelfModes"
          :key="m.key"
          class="shelf-card"
          :class="{ active: shelfMode === m.key, disabled: shelfSaving }"
          @click="changeShelfMode(m.key)"
        >
          <div class="shelf-card-head">
            <el-icon class="shelf-icon"><component :is="m.icon" /></el-icon>
            <el-icon v-if="shelfMode === m.key" class="shelf-check"><Check /></el-icon>
          </div>
          <div class="shelf-name">{{ m.label }}</div>
          <div class="shelf-desc">{{ m.desc }}</div>
        </div>
      </div>
      <div v-if="shelfMode === SHELF_MODE_SELECTED" class="shelf-tip">
        <el-icon :size="16"><InfoFilled /></el-icon>
        <span>该习惯会同步到当前账号的所有设备。书架只显示已添加的书籍，请到「分类管理」中挑选，新导入的图书同样需要手动添加。</span>
        <el-button size="small" text type="primary" @click="goCategories">去分类管理</el-button>
      </div>
    </div>

    <div class="pref-actions">
      <el-button :icon="RefreshLeft" @click="resetAll">恢复默认设置</el-button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { RefreshLeft } from '@element-plus/icons-vue'
import { loadReaderPrefs, saveReaderPrefs, resetReaderPrefs } from '@/utils/readerPrefs'
import {
  shelfMode as sharedShelfMode,
  ensureShelfModeLoaded,
  saveShelfMode,
  SHELF_MODE_ALL,
  SHELF_MODE_SELECTED
} from '@/utils/shelfPrefs'

const router = useRouter()
const prefs = ref(loadReaderPrefs())
// 账号级偏好：各页面共享同一状态，保存到服务端后多端一致
const shelfMode = computed(() => sharedShelfMode.value)
const shelfSaving = ref(false)
onMounted(ensureShelfModeLoaded)

const shelfModes = [
  {
    key: SHELF_MODE_ALL,
    icon: 'Collection',
    label: '显示全部书籍',
    desc: '书架展示账号内的全部图书，保持原有使用方式（默认）'
  },
  {
    key: SHELF_MODE_SELECTED,
    icon: 'CircleCheck',
    label: '指定书籍到书架',
    desc: '只展示从分类中挑选并加入书架的图书'
  }
]

const changeShelfMode = async (mode) => {
  if (shelfMode.value === mode || shelfSaving.value) return
  shelfSaving.value = true
  try {
    await saveShelfMode(mode)
    ElMessage.success(mode === SHELF_MODE_SELECTED ? '已切换为「指定书籍到书架」，所有设备同步生效' : '已切换为「显示全部书籍」，所有设备同步生效')
  } catch (e) {
    ElMessage.error('保存书架习惯失败，请检查网络后重试')
  } finally {
    shelfSaving.value = false
  }
}

const goCategories = () => router.push('/categories')

const themeList = [
  {
    key: 'light',
    label: '日间',
    swatchStyle: 'background: #ffffff;',
    lineColor: '#3a3f4a'
  },
  {
    key: 'sepia',
    label: '护眼',
    swatchStyle: 'background: #f7f1e3;',
    lineColor: '#6b5d43'
  },
  {
    key: 'dark',
    label: '夜间',
    swatchStyle: 'background: #26282e;',
    lineColor: '#9aa0ab'
  },
  {
    key: 'pitch',
    label: '纯黑',
    swatchStyle: 'background: #000000;',
    lineColor: '#7c8494'
  }
]

const fontOptions = [
  { label: '系统默认', value: 'system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif' },
  { label: '宋体', value: '"Songti SC", "SimSun", serif' },
  { label: '黑体', value: '"Heiti SC", "SimHei", sans-serif' },
  { label: '楷体', value: '"Kaiti SC", "KaiTi", serif' },
  { label: '仿宋', value: '"FangSong", "FangSong SC", serif' },
  { label: '等线', value: '"DengXian", "Microsoft YaHei", sans-serif' }
]

const setPref = (key, value) => {
  prefs.value = { ...prefs.value, [key]: value }
  // 只提交变化的字段，避免用页面快照覆盖其它偏好
  saveReaderPrefs({ [key]: value })
}

const resetAll = async () => {
  prefs.value = resetReaderPrefs()
  saveReaderPrefs(prefs.value)
  try {
    await saveShelfMode(SHELF_MODE_ALL)
    ElMessage.success('已恢复默认设置')
  } catch (e) {
    ElMessage.warning('阅读设置已恢复，但书架习惯同步失败，请重试')
  }
}
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

.pref-group {
  margin-bottom: 26px;
}
.pref-label {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  font-size: 14px;
  font-weight: 500;
}
.pref-value {
  font-size: 13px;
  color: var(--el-color-primary);
  font-variant-numeric: tabular-nums;
}

/* 主题卡片 */
.theme-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
.theme-card {
  cursor: pointer;
  border: 2px solid var(--el-border-color-lighter);
  border-radius: 12px;
  overflow: hidden;
  transition: all 0.2s;
}
.theme-card:hover {
  border-color: var(--el-color-primary-light-5);
}
.theme-card.active {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--app-primary) 15%, transparent);
}
.theme-swatch {
  height: 64px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.theme-line {
  height: 3px;
  border-radius: 2px;
  width: 100%;
  opacity: 0.55;
}
.theme-line.short {
  width: 65%;
}
.theme-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 10px;
  font-size: 13px;
  background: var(--el-fill-color-blank);
}
.theme-check {
  color: var(--el-color-primary);
  font-size: 15px;
}

/* 书架使用习惯 */
.shelf-cards {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}
.shelf-card {
  cursor: pointer;
  padding: 14px 16px;
  border: 2px solid var(--el-border-color-lighter);
  border-radius: 12px;
  transition: all 0.2s;
}
.shelf-card:hover {
  border-color: var(--el-color-primary-light-5);
}
.shelf-card.active {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--app-primary) 15%, transparent);
}
.shelf-card.disabled {
  opacity: 0.6;
  pointer-events: none;
}
.shelf-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.shelf-icon {
  font-size: 20px;
  color: var(--el-color-primary);
}
.shelf-check {
  color: var(--el-color-primary);
}
.shelf-name {
  margin-bottom: 4px;
  font-size: 14px;
  font-weight: 600;
}
.shelf-desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.shelf-tip {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.shelf-tip > .el-icon {
  flex-shrink: 0;
  color: var(--el-color-primary);
}
.shelf-tip > span {
  flex: 1;
}
.shelf-tip .el-button {
  flex-shrink: 0;
}

.pref-actions {
  margin-top: 8px;
  padding-top: 16px;
  border-top: 1px dashed var(--el-border-color-lighter);
}

@media (max-width: 768px) {
  .theme-cards {
    grid-template-columns: repeat(2, 1fr);
  }
  .shelf-cards {
    grid-template-columns: 1fr;
  }
}
</style>
