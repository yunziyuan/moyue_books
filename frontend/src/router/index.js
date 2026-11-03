import { createRouter, createWebHashHistory } from 'vue-router'
import { getToken } from '@/api/auth'
import { getServerUrl } from '@/utils/serverConfig'
import { isAndroidNative } from '@/utils/platform'
import Bookshelf from '@/views/Bookshelf.vue'
import ReaderEntry from '@/views/ReaderEntry.vue'
import Upload from '@/views/Upload.vue'
import BookDetail from '@/views/BookDetail.vue'
import Collections from '@/views/Collections.vue'
import Stats from '@/views/Stats.vue'
import Settings from '@/views/Settings.vue'
import Audiobooks from '@/views/Audiobooks.vue'
import Categories from '@/views/Categories.vue'
import LibraryDetail from '@/views/LibraryDetail.vue'
import LibraryHub from '@/views/LibraryHub.vue'
import Login from '@/views/Login.vue'
import SharePreview from '@/views/SharePreview.vue'
import AudiobookSharePreview from '@/views/AudiobookSharePreview.vue'
import ServerSetup from '@/views/ServerSetup.vue'

const routes = [
  { path: '/', redirect: '/bookshelf' },
  { path: '/login', name: 'Login', component: Login, meta: { public: true } },
  { path: '/share/:token', name: 'SharePreview', component: SharePreview, meta: { public: true } },
  { path: '/share/audio/:token', name: 'AudiobookSharePreview', component: AudiobookSharePreview, meta: { public: true } },
  { path: '/bookshelf', name: 'Bookshelf', component: Bookshelf },
  { path: '/upload', name: 'Upload', component: Upload },
  { path: '/book/:id', name: 'BookDetail', component: BookDetail, props: true },
  { path: '/reader/:id', name: 'Reader', component: ReaderEntry, props: true },
  { path: '/collections', name: 'Collections', component: Collections },
  { path: '/categories', name: 'Categories', component: Categories },
  { path: '/libraries', name: 'Libraries', component: LibraryHub },
  { path: '/library/:id', name: 'LibraryDetail', component: LibraryDetail, props: true },
  // 导入图书已整合进媒体库页面（tab 切换），这里重定向以保持旧链接 / 书架入口可用
  { path: '/upload', redirect: { path: '/libraries', query: { tab: 'import' } } },
  { path: '/audiobooks', name: 'Audiobooks', component: Audiobooks },
  { path: '/stats', name: 'Stats', component: Stats },
  { path: '/reviews', name: 'Reviews', component: () => import('@/views/Reviews.vue') },
  { path: '/reviews/editor', name: 'ReviewEditor', component: () => import('@/views/ReviewEditor.vue') },
  { path: '/settings', name: 'Settings', component: Settings },
  // 移动端 / 远程客户端首次启动：填写飞牛服务器地址（公开，可随时重配）
  { path: '/server-setup', name: 'ServerSetup', component: ServerSetup, meta: { public: true, setup: true } }
]

// hash 模式：兼容 App（WebView file:// 加载）与静态部署，刷新不 404
const router = createRouter({
  history: createWebHashHistory(),
  routes
})

// 全局登录守卫
router.beforeEach((to) => {
  const token = getToken()
  const server = getServerUrl()
  // 服务器连接配置页（/server-setup）仅安卓原生端使用；网页端不暴露该能力，
  // 直接访问时回首页，避免网页端出现「填写飞牛服务器地址」这类远程客户端专属界面。
  if (to.meta.setup && !isAndroidNative) {
    return { path: '/' }
  }
  // 未配置服务器地址（安卓远程客户端），且当前不是设置页 / 公开页 → 引导去设置。
  // 仅安卓端需要此引导：移动网页 / PC 网页由服务器同源托管，按网关前缀或同源 /api 访问，
  // 无需用户填写服务器地址，故不强制跳转。
  if (isAndroidNative && !server && !to.meta.public && !to.meta.setup) {
    return { path: '/server-setup' }
  }
  if (!to.meta.public && !token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.path === '/login' && token) {
    return { path: '/' }
  }
  return true
})

// 进入阅读页/详情页前记录"来时的路径"，让返回按钮能无视刷新导致的重复历史项，
// 直接跳回去（即使刷新过 N 次也只需 1 次点击）。仅记录从非本类页面进入的情况，
// 避免内部跳转（如目录跳转 / 阅读页返回）误覆盖前序路径。
// 白名单：只接受以下前缀的"来时路径"作为合法目标，避免分享页/登录页等污染
const VALID_PREV_PREFIXES = ['/bookshelf', '/upload', '/book/', '/reader/',
  '/collections', '/categories', '/audiobooks', '/stats',
  '/reviews', '/settings', '/libraries', '/library/']

router.beforeEach((to, from) => {
  // 阅读页：捕获来路（避免 router.back 落到重复 reader entry 上）
  if (to.name === 'Reader' && from.name !== 'Reader') {
    const ok = from.fullPath && VALID_PREV_PREFIXES.some(p => from.fullPath.startsWith(p))
    try { sessionStorage.setItem('moyue_prev_reader_path', ok ? from.fullPath : '/bookshelf') } catch {}
  }
  // 详情页：捕获来路（BookDetail 的返回按钮直接跳回对应菜单，而非固定回书架）
  // 注意：必须排除从 Reader 来的情况，否则 Reader 用 replace 返回 BookDetail 时
  // 会把 prev_bookdetail 覆盖成 /reader/:id，导致 BookDetail 返回按钮又跳回 Reader
  if (to.name === 'BookDetail' && from.name !== 'BookDetail' && from.name !== 'Reader') {
    const ok = from.fullPath && VALID_PREV_PREFIXES.some(p => from.fullPath.startsWith(p))
    try { sessionStorage.setItem('moyue_prev_bookdetail_path', ok ? from.fullPath : '/bookshelf') } catch {}
  }
})

export default router
