import { Capacitor } from '@capacitor/core'

// 是否运行在安卓原生端（Capacitor 打包的 App）。
// 仅安卓端是「远程客户端」，需要用户填写飞牛服务器地址并可在多服务器间切换；
// 移动网页 / PC 网页由服务器同源托管（或走网关前缀），无需、也不应暴露「服务器连接」配置。
// 注意：Capacitor.isNativePlatform() 在 web 端恒为 false，getPlatform() 返回 'web'，
// 因此该常量在移动网页 / PC 网页端均为 false。
export const isAndroidNative =
  Capacitor.isNativePlatform() && Capacitor.getPlatform() === 'android'
