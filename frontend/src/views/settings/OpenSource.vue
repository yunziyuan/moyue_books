<template>
  <div class="settings-section opensource-section">
    <div class="section-title">
      <el-icon><Link /></el-icon>
      <div>
        <h3>开源说明</h3>
        <p>墨阅图书是一款完全开源的个人图书管理与阅读应用</p>
      </div>
    </div>

    <div class="os-card">
      <div class="os-header">
        <el-icon class="os-icon"><Connection /></el-icon>
        <div>
          <h4>开源协议与说明</h4>
          <p>本项目基于 MIT 协议开源，可自由使用、修改与分发</p>
        </div>
      </div>
      <ul class="os-list">
        <li><el-icon><Check /></el-icon> 基于 <strong>MIT 许可证</strong> 发布，可自由用于商业与非商业用途</li>
        <li><el-icon><Check /></el-icon> 后端 Go + Gin + GORM + SQLite，前端 Vue 3 + Element Plus + Vite</li>
        <li><el-icon><Check /></el-icon> 支持自部署、二次开发，欢迎 Fork 与提交贡献</li>
        <li><el-icon><Check /></el-icon> 软件按「原样」提供，作者不承担任何担保或责任</li>
      </ul>
    </div>

    <div class="os-card">
      <div class="os-header">
        <el-icon class="os-icon repo"><Star /></el-icon>
        <div>
          <h4>开源地址</h4>
          <p>代码托管于 GitHub，欢迎 Star、Issue 与 Pull Request</p>
        </div>
      </div>
      <div class="os-repo">
        <div class="os-repo-info">
          <el-icon class="os-repo-ic"><Position /></el-icon>
          <a :href="repoUrl" target="_blank" rel="noopener">{{ repoUrl }}</a>
        </div>
        <div class="os-repo-actions">
          <el-button type="primary" :icon="Star" @click="openRepo">前往 Star</el-button>
          <el-button :icon="Download" @click="copyUrl">复制地址</el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ElMessage } from 'element-plus'
import { Link, Connection, Star, Check, Position, Download } from '@element-plus/icons-vue'

const repoUrl = 'https://github.com/yunziyuan/moyue_books'

const openRepo = () => window.open(repoUrl, '_blank')

const copyUrl = async () => {
  try {
    await navigator.clipboard.writeText(repoUrl)
    ElMessage.success('已复制开源地址')
  } catch {
    ElMessage.error('复制失败，请手动复制')
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

.os-card {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 16px;
  background: var(--el-bg-color);
  box-shadow: 0 8px 24px rgba(31, 38, 135, 0.06);
  padding: 20px 22px;
  margin-bottom: 16px;
}
.os-card:last-child {
  margin-bottom: 0;
}
.os-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}
.os-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  font-size: 22px;
  color: #fff;
  flex: none;
  justify-content: center;
  background: linear-gradient(135deg, #10b981, #059669);
}
.os-icon.repo {
  background: linear-gradient(135deg, #f59e0b, #d97706);
}
.os-header h4 {
  margin: 0;
  font-size: 15px;
}
.os-header p {
  margin: 3px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.os-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.os-list li {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  color: var(--el-text-color-regular);
  line-height: 1.5;
}
.os-list li :deep(.el-icon) {
  color: #10b981;
  margin-top: 2px;
  flex: none;
}
.os-list li strong {
  color: var(--el-color-primary);
}

.os-repo {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.os-repo-info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.os-repo-ic {
  color: var(--el-color-primary);
  flex: none;
}
.os-repo-info a {
  color: var(--el-color-primary);
  text-decoration: none;
  font-size: 14px;
  font-weight: 600;
  word-break: break-all;
}
.os-repo-info a:hover {
  text-decoration: underline;
}
.os-repo-actions {
  display: flex;
  gap: 10px;
  flex: none;
}

@media (max-width: 768px) {
  .os-repo {
    flex-direction: column;
    align-items: stretch;
  }
  .os-repo-actions .el-button {
    flex: 1;
  }
}
</style>
