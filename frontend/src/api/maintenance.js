import axios from 'axios'
import { API_BASE } from '@/config'

// 数据维护：一键去重复书架及无效数据
export const maintenanceApi = {
  // dryRun=true 仅统计待清理数量（预览），false 执行清理
  cleanup(dryRun = false) {
    const q = dryRun ? '?dry_run=1' : ''
    return axios.post(`${API_BASE}/maintenance/cleanup${q}`)
  }
}

export default maintenanceApi
