import './polyfills.js' // 旧版 WebView 兼容：Object.groupBy / Map.groupBy（foliate-js 依赖）
import { createApp } from 'vue'
import axios from 'axios'
import { Capacitor } from '@capacitor/core'
import { capacitorAxiosAdapter } from '@/utils/nativeHttp'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import 'element-plus/dist/index.css'
import './styles/dialog.css'
import './styles/pagination.css'
import App from './App.vue'
import router from './router'
import { getToken, clearToken, setUser } from './api/auth'
import { applyAppPrefs } from './utils/appPrefs'
import { installNativeImageLoader } from '@/utils/nativeImage'

// axios 全局拦截器：自动携带 token，401 时跳转登录页
axios.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

axios.interceptors.response.use(
  (response) => response,
  (error) => {
    // hash 路由：登录页路径为 #/login，跳转需带 BASE_URL 前缀（兼容飞牛网关 /app/moyue 部署）
    if (error.response?.status === 401 && !window.location.hash.startsWith('#/login')) {
      clearToken()
      setUser(null)
      const redirect = encodeURIComponent(window.location.hash.slice(1) || '/bookshelf')
      window.location.replace(`${import.meta.env.BASE_URL}#/login?redirect=${redirect}`)
    }
    return Promise.reject(error)
  }
)

// 移动端：让所有 axios 请求走原生层(HttpPlugin)，绕开 WebView 的私有网络访问(PNA)拦截
if (Capacitor.isNativePlatform()) {
  axios.defaults.adapter = capacitorAxiosAdapter
}

const app = createApp(App)

// 启动时应用全局外观配置（主题色 / 网站名称 / 网站图标 / 动效开关）
applyAppPrefs()

// 安卓端：封面 / EPUB 插图等跨源图片改由原生 HTTP 通道取回，绕开 WebView 的混合内容与 PNA 拦截
installNativeImageLoader()

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus, { locale: zhCn })
app.use(router)
app.mount('#app')
