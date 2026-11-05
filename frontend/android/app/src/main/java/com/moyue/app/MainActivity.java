package com.moyue.app;

import android.os.Bundle;
import android.view.ViewGroup;
import android.webkit.WebSettings;
import android.webkit.WebView;

import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;

import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        // 注册本地原生 HTTP 插件：源码位于本模块(com.moyue.app)，Capacitor 不会自动发现，
        // 必须通过 BridgeActivity.registerPlugin 在 super.onCreate 之前登记，init 时一并加载。
        // 该插件在原生层(HttpURLConnection)发请求，绕开 WebView 对跨私有网络请求的 PNA 拦截。
        registerPlugin(HttpPlugin.class);
        super.onCreate(savedInstanceState);

        // 让原生层代理 WebView 对后端主机(http/https)的图片等子资源请求，
        // 彻底绕开 Android WebView 对「从 capacitor://localhost 跨私有网络 / 混合内容」的拦截，
        // 解决安卓端封面 / 阅读器插图等图片无法显示的问题。
        // 同时放开混合内容限制，是 ProxyWebViewClient 能接管请求的前置条件。
        Bridge bridge = getBridge();
        if (bridge != null) {
            WebView webView = bridge.getWebView();
            // 用 Bridge.setWebViewClient 同步更新 Bridge 内部持有的 client，
            // 否则 getWebViewClient() 仍指向旧对象，插件/本地服务可能绕过代理。
            bridge.setWebViewClient(new ProxyWebViewClient(bridge));
            WebSettings settings = webView.getSettings();
            settings.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
            settings.setAllowFileAccess(true);
            settings.setAllowUniversalAccessFromFileURLs(true);
        }

        // targetSdk 35+ 强制 edge-to-edge：系统不再为状态栏/手势条预留空间，
        // 需要把系统栏 inset 转为内容容器(android.R.id.content)的内边距，
        // 使 WebView 从状态栏下方开始，避免顶部内容与信号/时间重叠。
        // CONSUMED 阻断向 WebView 分发，CSS env(safe-area-inset-*) 恒为 0，由原生统一兜底。
        ViewCompat.setOnApplyWindowInsetsListener(findViewById(android.R.id.content), (v, windowInsets) -> {
            Insets bars = windowInsets.getInsets(
                    WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.displayCutout());
            ViewGroup content = (ViewGroup) v;
            content.setPadding(bars.left, bars.top, bars.right, bars.bottom);
            return WindowInsetsCompat.CONSUMED;
        });
    }
}
