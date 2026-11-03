package com.moyue.app;

import com.getcapacitor.JSArray;
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
import java.util.UUID;

import org.json.JSONArray;
import org.json.JSONObject;

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

    /**
     * 原生层 multipart/form-data 上传，绕开 WebView 对跨私有网络请求的 PNA 拦截。
     * 入参：
     *   url      : 目标地址
     *   headers  : 请求头(map)
     *   fields   : 文本字段(map)
     *   files    : 文件数组，每项 { field, filename, contentType, data(base64) }
     * 文件内容以 base64 由前端传入（受 JS<->原生桥序列化限制），适合普通体积（封面/常规书籍）。
     */
    @PluginMethod
    public void upload(PluginCall call) {
        String url = call.getString("url");
        if (url == null) {
            call.reject("url required");
            return;
        }
        JSObject headersObj = call.getObject("headers", new JSObject());
        JSObject fieldsObj = call.getObject("fields", new JSObject());
        JSArray filesJs = call.getArray("files", new JSArray());
        JSONArray filesArr = filesJs;

        try {
            HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setConnectTimeout(60000);
            conn.setReadTimeout(60000);

            Iterator<String> hit = headersObj.keys();
            while (hit.hasNext()) {
                String k = hit.next();
                String v = headersObj.optString(k, null);
                if (v != null) conn.setRequestProperty(k, v);
            }

            String boundary = "----MoyueBoundary" + UUID.randomUUID().toString().replace("-", "");
            conn.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);

            ByteArrayOutputStream body = new ByteArrayOutputStream();
            byte[] delim = ("--" + boundary + "\r\n").getBytes("UTF-8");

            Iterator<String> fit = fieldsObj.keys();
            while (fit.hasNext()) {
                String name = fit.next();
                String value = fieldsObj.optString(name, "");
                body.write(delim);
                body.write(("Content-Disposition: form-data; name=\"" + name + "\"\r\n\r\n").getBytes("UTF-8"));
                body.write(value.getBytes("UTF-8"));
                body.write("\r\n".getBytes("UTF-8"));
            }

            for (int i = 0; i < filesArr.length(); i++) {
                JSONObject f = filesArr.getJSONObject(i);
                String field = f.optString("field", "file");
                String filename = f.optString("filename", "file");
                String contentType = f.optString("contentType", "application/octet-stream");
                String b64 = f.optString("data", "");
                byte[] bytes = android.util.Base64.decode(b64, android.util.Base64.DEFAULT);
                body.write(delim);
                body.write(("Content-Disposition: form-data; name=\"" + field
                        + "\"; filename=\"" + filename + "\"\r\n").getBytes("UTF-8"));
                body.write(("Content-Type: " + contentType + "\r\n\r\n").getBytes("UTF-8"));
                body.write(bytes);
                body.write("\r\n".getBytes("UTF-8"));
            }
            body.write(("--" + boundary + "--\r\n").getBytes("UTF-8"));

            byte[] payload = body.toByteArray();
            conn.setFixedLengthStreamingMode(payload.length);
            try (OutputStream os = conn.getOutputStream()) {
                os.write(payload);
            }

            int status = conn.getResponseCode();
            InputStream in = status >= 400 ? conn.getErrorStream() : conn.getInputStream();
            byte[] resp = readAll(in);

            JSObject ret = new JSObject();
            ret.put("status", status);
            ret.put("data", new String(resp, "UTF-8"));
            call.resolve(ret);
            conn.disconnect();
        } catch (Exception e) {
            call.reject(e.getMessage());
        }
    }
}
