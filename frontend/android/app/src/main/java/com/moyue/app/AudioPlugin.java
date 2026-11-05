package com.moyue.app;

import android.content.Intent;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

/**
 * 有声书播放器 JS 桥：把 JS 层的播放指令转成 Intent 交给 AudioPlayerService，
 * 并把服务的播放事件通过 notifyListeners("audioEvent") 推回 JS。
 *
 * 前端对应 src/player/nativeAudio.js，注册见 MainActivity.registerPlugin(AudioPlugin.class)。
 */
@CapacitorPlugin(name = "AudioPlugin")
public class AudioPlugin extends Plugin implements AudioPlayerService.Emitter {

    public AudioPlugin() {
        AudioPlayerService.setEmitter(this);
    }

    @Override
    protected void handleOnDestroy() {
        AudioPlayerService.setEmitter(null);
        super.handleOnDestroy();
    }

    @Override
    public void emit(JSObject event) {
        notifyListeners("audioEvent", event);
    }

    private void dispatch(String action, PluginCall call) {
        Intent intent = new Intent(getContext(), AudioPlayerService.class).setAction(action);
        if (call != null) {
            JSObject data = call.getData();
            if (data != null) {
                intent.putExtra(AudioPlayerService.EXTRA_URL, data.optString("url", ""));
                intent.putExtra(AudioPlayerService.EXTRA_POSITION, data.optDouble("position", 0));
                intent.putExtra(AudioPlayerService.EXTRA_SECONDS, data.optDouble("seconds", 0));
                intent.putExtra(AudioPlayerService.EXTRA_TITLE, data.optString("title", ""));
                intent.putExtra(AudioPlayerService.EXTRA_SUBTITLE, data.optString("subtitle", ""));
            }
        }
        AudioPlayerService.send(getContext(), intent);
    }

    @PluginMethod
    public void load(PluginCall call) {
        if (call.getString("url") == null) {
            call.reject("url required");
            return;
        }
        dispatch(AudioPlayerService.ACTION_LOAD, call);
        call.resolve();
    }

    @PluginMethod
    public void play(PluginCall call) {
        dispatch(AudioPlayerService.ACTION_PLAY, null);
        call.resolve();
    }

    @PluginMethod
    public void pause(PluginCall call) {
        dispatch(AudioPlayerService.ACTION_PAUSE, null);
        call.resolve();
    }

    @PluginMethod
    public void stop(PluginCall call) {
        dispatch(AudioPlayerService.ACTION_STOP, null);
        call.resolve();
    }

    @PluginMethod
    public void seek(PluginCall call) {
        dispatch(AudioPlayerService.ACTION_SEEK, call);
        call.resolve();
    }

    @PluginMethod
    public void setMetadata(PluginCall call) {
        dispatch(AudioPlayerService.ACTION_METADATA, call);
        call.resolve();
    }

    /** Activity 重建 / 从后台恢复时，主动拉取一次原生播放器状态，避免事件丢失。 */
    @PluginMethod
    public void getPlaybackState(PluginCall call) {
        call.resolve(AudioPlayerService.state());
    }
}
