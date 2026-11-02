<template>
  <!-- 桌面端：保持完整分页 -->
  <el-pagination
    v-if="!isMobile"
    v-model:current-page="current"
    v-model:page-size="size"
    :total="total"
    :page-sizes="pageSizes"
    layout="total, sizes, prev, pager, next, jumper"
    background
    @current-change="(p) => emit('current-change', p)"
    @size-change="(s) => emit('size-change', s)"
  />

  <!-- 手机端：向上滚动加载后续（无限滚动），不使用按钮分页 -->
  <div v-else class="mscroll">
    <div ref="sentinel" class="mscroll-sentinel" />
    <div v-if="loadingMore" class="mscroll-status">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>正在加载…</span>
    </div>
    <div v-else-if="!hasMore" class="mscroll-status mscroll-end">没有更多了</div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { useIsMobile } from '@/composables/useMobileView'

const props = defineProps({
  currentPage: { type: Number, required: true },
  pageSize: { type: Number, required: true },
  total: { type: Number, required: true },
  pageSizes: { type: Array, default: () => [10, 20, 50, 100] },
  // 手机端无限滚动相关
  hasMore: { type: Boolean, default: true },
  loadingMore: { type: Boolean, default: false }
})
const emit = defineEmits([
  'update:currentPage',
  'update:pageSize',
  'current-change',
  'size-change',
  'load-more'
])

const { isMobile } = useIsMobile()

const current = computed({
  get: () => props.currentPage,
  set: (v) => emit('update:currentPage', v)
})
const size = computed({
  get: () => props.pageSize,
  set: (v) => emit('update:pageSize', v)
})

// ===== 手机端无限滚动 =====
const sentinel = ref(null)
let observer = null

const sentinelVisible = () => {
  const el = sentinel.value
  if (!el) return false
  const r = el.getBoundingClientRect()
  const vh = window.innerHeight || document.documentElement.clientHeight
  return r.top <= vh + 300
}

const tryLoadMore = () => {
  if (!isMobile.value) return
  if (!props.hasMore || props.loadingMore) return
  emit('load-more')
}

const setupObserver = () => {
  if (observer) observer.disconnect()
  if (!sentinel.value) return
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) tryLoadMore()
    },
    { rootMargin: '300px 0px' }
  )
  observer.observe(sentinel.value)
}

// 加载结束后若哨兵仍在视口内，继续加载直至填满屏幕
watch(
  () => props.loadingMore,
  (now, prev) => {
    if (prev && !now && isMobile.value && props.hasMore) {
      nextTick(() => {
        if (sentinelVisible()) emit('load-more')
      })
    }
  }
)
// 数据变化（如筛选后内容变短，哨兵重新进入视口）时触发首次加载
watch(
  () => props.hasMore,
  (v) => {
    if (v && isMobile.value) nextTick(() => { if (sentinelVisible()) emit('load-more') })
  }
)
watch(isMobile, (v) => {
  if (v) nextTick(setupObserver)
  else if (observer) observer.disconnect()
})

onMounted(() => {
  if (isMobile.value) nextTick(setupObserver)
})
onBeforeUnmount(() => {
  if (observer) observer.disconnect()
})
</script>

<style scoped>
.mscroll {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
  padding: 16px 0 8px;
}
.mscroll-sentinel {
  width: 100%;
  height: 1px;
}
.mscroll-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #94a3b8;
}
.mscroll-end {
  color: #cbd5e1;
}
</style>
