<template>
  <div class="settings-section data-cleanup">
    <div class="section-title">
      <el-icon><Delete /></el-icon>
      <div>
        <h3>数据清理</h3>
        <p>一键去重复书架，清理源文件缺失、空分类及孤儿记录等无效数据</p>
      </div>
    </div>

    <el-alert
      class="intro"
      type="info"
      :closable="false"
      show-icon
    >
      <template #title>清理范围说明</template>
      <ul class="intro-list">
        <li><b>重复图书</b>：同一文件路径多次入库的副本，保留阅读进度最高 / 最近更新的一份，其余合并批注书签后删除。</li>
        <li><b>无效数据</b>：源文件已消失的图书、没有任何图书的空分类、指向已删除图书的孤儿批注 / 书签 / 读后感 / 阅读历史 / 收藏关联。</li>
      </ul>
    </el-alert>

    <div class="actions">
      <el-button :icon="Search" :loading="previewing" @click="onPreview">扫描预览</el-button>
      <el-button
        type="danger"
        :icon="Delete"
        :disabled="!hasIssues"
        :loading="cleaning"
        @click="onClean"
      >立即清理</el-button>
    </div>

    <el-alert
      v-if="previewed && !hasIssues && !cleaning"
      class="result"
      type="success"
      :closable="false"
      show-icon
      title="未发现需要清理的重复或无效数据，书架状态良好。"
    />

    <div v-if="report" class="report card">
      <div class="report-head">
        <span class="report-title">{{ report.dry_run ? '预览结果（尚未删除）' : '清理完成' }}</span>
        <el-tag v-if="!report.dry_run" type="success" effect="light" size="small">已执行</el-tag>
      </div>
      <div class="report-grid">
        <div v-for="item in reportItems" :key="item.key" class="report-item">
          <span class="ri-label">{{ item.label }}</span>
          <span class="ri-value" :class="{ zero: item.value === 0 }">{{ item.value }}</span>
        </div>
      </div>
    </div>

    <el-alert
      v-if="error"
      class="result"
      type="error"
      :closable="false"
      show-icon
      :title="error"
    />
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Delete, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import maintenanceApi from '@/api/maintenance'

const previewing = ref(false)
const cleaning = ref(false)
const previewed = ref(false)
const report = ref(null)
const error = ref('')

// 各清理类型在报告中的展示顺序与中文标签
const labels = {
  duplicate_books: '重复图书（已合并删除）',
  missing_source_books: '源文件缺失图书',
  empty_categories: '空分类',
  orphan_annotations: '孤儿批注',
  orphan_bookmarks: '孤儿书签',
  orphan_reviews: '孤儿读后感',
  orphan_reading_history: '孤儿阅读历史',
  orphan_collection_books: '孤儿收藏关联'
}

const reportItems = computed(() => {
  if (!report.value) return []
  return Object.keys(labels).map((k) => ({ key: k, label: labels[k], value: report.value[k] || 0 }))
})

const hasIssues = computed(() => {
  if (!report.value) return false
  return reportItems.value.some((i) => i.value > 0)
})

async function onPreview() {
  error.value = ''
  previewing.value = true
  report.value = null
  previewed.value = false
  try {
    const { data } = await maintenanceApi.cleanup(true)
    data.dry_run = true
    report.value = data
    previewed.value = true
  } catch (e) {
    error.value = e?.response?.data?.error || e?.message || '预览失败'
  } finally {
    previewing.value = false
  }
}

async function onClean() {
  error.value = ''
  if (!report.value) {
    await onPreview()
  }
  if (!hasIssues.value) {
    ElMessage.info('没有需要清理的数据')
    return
  }
  try {
    await ElMessageBox.confirm(
      '将删除重复图书副本、源文件缺失的图书及孤儿记录，且不可恢复。是否继续？',
      '确认清理',
      { type: 'warning', confirmButtonText: '立即清理', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  cleaning.value = true
  try {
    const { data } = await maintenanceApi.cleanup(false)
    report.value = data
    previewed.value = true
    const total =
      data.duplicate_books + data.missing_source_books + data.empty_categories +
      data.orphan_annotations + data.orphan_bookmarks + data.orphan_reviews +
      data.orphan_reading_history + data.orphan_collection_books
    ElMessage.success(`清理完成，共处理 ${total} 项无效数据`)
  } catch (e) {
    error.value = e?.response?.data?.error || e?.message || '清理失败'
  } finally {
    cleaning.value = false
  }
}
</script>

<style scoped>
.data-cleanup { text-align: left; }
.intro {
  margin-bottom: 18px;
  text-align: left;
}
.intro-list {
  margin: 6px 0 0;
  padding-left: 18px;
  line-height: 1.7;
  font-size: 13px;
  color: #475569;
}
.intro-list b { color: #1e293b; }
.actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.result { margin-top: 4px; }

.report {
  margin-top: 4px;
  padding: 18px 20px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.25);
}
.report-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}
.report-title {
  font-size: 15px;
  font-weight: 700;
  color: #0f172a;
}
.report-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 10px;
}
.report-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 10px;
  background: rgba(148, 163, 184, 0.08);
}
.ri-label {
  font-size: 12px;
  color: #64748b;
}
.ri-value {
  font-size: 22px;
  font-weight: 800;
  color: var(--app-primary);
}
.ri-value.zero {
  color: #94a3b8;
  font-weight: 600;
}
</style>
