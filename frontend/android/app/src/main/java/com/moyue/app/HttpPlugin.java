package com.moyue.app;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

import android.util.Base64;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.Iterator;
import java.util.List;
import java.util.Map;

/**
 * 原生 HTTP 插件：在原生层(Java HttpURLConnection)发起请求，
 * 彻底绕开 Android WebView 对「跨私有网络(如 localhost→192.168.x.x)」请求的 PNA 拦截。
 */
@CapacitorPlugin(name = "HttpPlugin")
public class HttpPlugin extends Plugin {

    @PluginMethod
    public void request(PluginCall call) {
        String method = call.getString("method", "GET");
        String url = call.getString("url");
        if (url == null) {
            call.reject("url required");
            return;
        }
        JSObject headersObj = call.getObject("headers", new JSObject());
        String data = call.getString("data", null);
        String responseType = call.getString("responseType", "json");
        int timeout = call.getInt("timeout", 0);

        try {
            HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
            conn.setRequestMethod(method.toUpperCase());
            conn.setInstanceFollowRedirects(true);
            conn.setConnectTimeout(timeout > 0 ? timeout : 15000);
            conn.setReadTimeout(timeout > 0 ? timeout : 15000);

            Iterator<String> it = headersObj.keys();
            while (it.hasNext()) {
                String k = it.next();
                String v = headersObj.optString(k, null);
                if (v != null) conn.setRequestProperty(k, v);
            }

            if (data != null && !method.equalsIgnoreCase("GET") && !method.equalsIgnoreCase("HEAD")) {
                conn.setDoOutput(true);
                byte[] bytes = data.getBytes("UTF-8");
                conn.setFixedLengthStreamingMode(bytes.length);
                try (OutputStream os = conn.getOutputStream()) {
                    os.write(bytes);
                }
            }

            int status = conn.getResponseCode();
            InputStream in = status >= 400 ? conn.getErrorStream() : conn.getInputStream();
            byte[] body = readAll(in);

            JSObject ret = new JSObject();
            ret.put("status", status);
            ret.put("statusText", conn.getResponseMessage());
            JSObject hdrs = new JSObject();
            for (Map.Entry<String, List<String>> e : conn.getHeaderFields().entrySet()) {
                if (e.getKey() != null && !e.getValue().isEmpty()) {
                    hdrs.put(e.getKey(), e.getValue().get(0));
                }
            }
            ret.put("headers", hdrs);

            if ("arraybuffer".equals(responseType) || "blob".equals(responseType)) {
                ret.put("data", Base64.encodeToString(body, Base64.NO_WRAP));
            } else {
                ret.put("data", new String(body, "UTF-8"));
            }
            call.resolve(ret);
            conn.disconnect();
        } catch (IOException e) {
            call.reject(e.getMessage());
        }
    }

    private byte[] readAll(InputStream in) throws IOException {
        ByteArrayOutputStream baos = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) != -1) {
            baos.write(buf, 0, n);
        }
        return baos.toByteArray();
    }
}
