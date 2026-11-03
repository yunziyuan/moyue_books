package com.moyue.app;

import android.annotation.TargetApi;
import android.os.Build;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;

import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeWebViewClient;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 代理 WebView 对后端主机(http/https)的子资源请求（图片/字体/CSS 等）。
 *
 * 安卓 WebView 运行在 capacitor://localhost 下，直接加载 http://私有IP 的资源会被
 * 「混合内容」与「私有网络访问(PNA)」策略拦截，导致封面、阅读器插图等图片无法显示。
 * 此处在原生层用 HttpURLConnection 代为请求并回灌给 WebView，
 * 请求完全不经过 WebView 的网络栈，从而绕开上述拦截。
 *
 * 仅代理非 localhost 的 http/https GET/HEAD 请求；其余请求（含 Capacitor 本地服务、
 * bridge 消息、POST 上传等）一律交给 super 处理，不影响 Capacitor 既有行为。
 */
public class ProxyWebViewClient extends BridgeWebViewClient {

    public ProxyWebViewClient(Bridge bridge) {
        super(bridge);
    }

    @Override
    public WebResourceResponse shouldInterceptRequest(WebView view, String url) {
        WebResourceResponse r = maybeProxy(url, null, "GET");
        return r != null ? r : super.shouldInterceptRequest(view, url);
    }

    @TargetApi(Build.VERSION_CODES.LOLLIPOP)
    @Override
    public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
        String url = request.getUrl() != null ? request.getUrl().toString() : null;
        if (url == null) return super.shouldInterceptRequest(view, request);
        WebResourceResponse r = maybeProxy(url, request.getRequestHeaders(), request.getMethod());
        return r != null ? r : super.shouldInterceptRequest(view, request);
    }

    private WebResourceResponse maybeProxy(String url, Map<String, String> headers, String method) {
        if (url == null) return null;

        String methodUpper = method != null ? method.toUpperCase() : "GET";
        // 仅代理对后端主机(http/https)的请求；其余(含 Capacitor 本地服务、bridge)交 super
        URL u;
        try {
            u = new URL(url);
        } catch (Exception e) {
            return null;
        }
        String scheme = u.getProtocol();
        if (!"http".equalsIgnoreCase(scheme) && !"https".equalsIgnoreCase(scheme)) return null;

        String host = u.getHost();
        if (host == null) return null;
        host = host.toLowerCase();
        // 跳过本地/回环地址，交给 Capacitor 本地服务与 super 处理
        if ("localhost".equals(host) || "127.0.0.1".equals(host) || host.endsWith(".local")) {
            return null;
        }

        // 放行 PNA/CORS 预检：从 capacitor://localhost 访问私有 IP 时，WebView 会先发 OPTIONS 预检，
        // 后端(Go)不回 Access-Control-Allow-Private-Network，导致整次请求被拦截、图片等子资源加载失败。
        // 这里直接合成放行响应，使后续真正的 GET/POST 能发出，并被本代理接管。
        if ("OPTIONS".equals(methodUpper)) {
            Map<String, String> ph = new HashMap<>();
            String origin = headers != null ? headers.get("Origin") : null;
            ph.put("Access-Control-Allow-Origin", origin != null ? origin : "*");
            ph.put("Access-Control-Allow-Private-Network", "true");
            ph.put("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD, PUT, DELETE");
            ph.put("Access-Control-Allow-Headers", "*");
            ph.put("Access-Control-Max-Age", "86400");
            return new WebResourceResponse(
                    "text/plain", "UTF-8", 200, "OK", ph, new ByteArrayInputStream(new byte[0]));
        }

        if (!"GET".equals(methodUpper) && !"HEAD".equals(methodUpper)) return null;

        try {
            HttpURLConnection conn = (HttpURLConnection) u.openConnection();
            conn.setRequestMethod(methodUpper);
            conn.setInstanceFollowRedirects(true);
            conn.setConnectTimeout(15000);
            conn.setReadTimeout(30000);
            if (headers != null) {
                for (Map.Entry<String, String> e : headers.entrySet()) {
                    String k = e.getKey();
                    if (k == null || e.getValue() == null) continue;
                    // 这些头由连接自行处理，原样透传可能干扰代理请求
                    if (k.equalsIgnoreCase("Host")
                            || k.equalsIgnoreCase("Connection")
                            || k.equalsIgnoreCase("Content-Length")
                            || k.equalsIgnoreCase("Transfer-Encoding")
                            || k.equalsIgnoreCase("Accept-Encoding")) {
                        continue;
                    }
                    conn.setRequestProperty(k, e.getValue());
                }
            }

            int status = conn.getResponseCode();
            InputStream in = status >= 400 ? conn.getErrorStream() : conn.getInputStream();
            byte[] body = readAll(in);

            String ct = conn.getContentType();
            String mime = ct != null ? ct : "application/octet-stream";
            int sc = mime.indexOf(';');
            if (sc >= 0) mime = mime.substring(0, sc).trim();
            if (mime.isEmpty()) mime = "application/octet-stream";

            // 文本类资源透传编码；二进制(图片等)传 null，避免 WebView 误当作文本解码
            String encoding = null;
            if (mime.startsWith("text/")
                    || mime.equals("application/json")
                    || mime.equals("application/xml")
                    || mime.equals("application/javascript")) {
                encoding = "UTF-8";
            }

            Map<String, String> respHeaders = new HashMap<>();
            Map<String, List<String>> rawHeaders = conn.getHeaderFields();
            if (rawHeaders != null) {
                for (Map.Entry<String, List<String>> e : rawHeaders.entrySet()) {
                    if (e.getKey() == null || e.getValue() == null || e.getValue().isEmpty()) continue;
                    String key = e.getKey();
                    if (key.equalsIgnoreCase("Content-Encoding")
                            || key.equalsIgnoreCase("Content-Length")
                            || key.equalsIgnoreCase("Transfer-Encoding")
                            || key.equalsIgnoreCase("Connection")) {
                        continue;
                    }
                    respHeaders.put(key, e.getValue().get(0));
                }
            }
            // 便于以 CORS 模式拉取的子资源(如阅读器内插图)在原生代理下也能成功
            respHeaders.put("Access-Control-Allow-Origin", "*");

            String reason = conn.getResponseMessage();
            conn.disconnect();
            return new WebResourceResponse(
                    mime,
                    encoding,
                    status,
                    reason,
                    respHeaders,
                    new ByteArrayInputStream(body));
        } catch (IOException | RuntimeException e) {
            // 代理失败则放弃，交由 super（请求会按原生策略失败），不影响其它资源
            return null;
        }
    }

    private byte[] readAll(InputStream in) throws IOException {
        if (in == null) return new byte[0];
        ByteArrayOutputStream baos = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) != -1) {
            baos.write(buf, 0, n);
        }
        return baos.toByteArray();
    }
}
