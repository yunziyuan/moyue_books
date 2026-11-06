package com.moyue.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.ServiceInfo;
import android.media.AudioAttributes;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.media.MediaMetadata;
import android.media.MediaPlayer;
import android.media.session.MediaSession;
import android.media.session.PlaybackState;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.PowerManager;
import android.util.Log;

import androidx.core.app.ServiceCompat;
import androidx.core.content.ContextCompat;

import com.getcapacitor.JSObject;

/**
 * 有声书原生播放器（前台服务）。
 *
 * 为什么需要它：
 * - WebView 的 <audio> 请求由媒体栈直接发往跨源 http://私有IP，绕开了 HttpPlugin / ProxyWebViewClient
 *   这套原生代理，会被 PNA / 混合内容策略拦截；原生 MediaPlayer 直连则没有这个问题。
 * - WebView 音频在 Activity 进入后台后会被系统挂起，只有前台服务 + MediaSession 才能稳定后台播放、
 *   响应耳机/蓝牙媒体键。
 *
 * 播放列表与进度仍由 JS 层（src/player/index.js）掌控：本服务只负责「当前一段音频」的解码播放，
 * 通过 AudioPlugin 把 ready/timeupdate/ended/error 事件回传给 JS，由 JS 决定下一段。
 * 播放期间 MediaPlayer 持有 PARTIAL_WAKE_LOCK，保证后台收到 ended 后能自动续播。
 */
public class AudioPlayerService extends Service {

    private static final String TAG = "MoyueAudio";
    private static final int NOTIFICATION_ID = 1001;
    private static final String CHANNEL_ID = "moyue_audiobook";
    private static final long TICK_MS = 1000L;

    static final String ACTION_LOAD = "com.moyue.app.audio.LOAD";
    static final String ACTION_PLAY = "com.moyue.app.audio.PLAY";
    static final String ACTION_PAUSE = "com.moyue.app.audio.PAUSE";
    static final String ACTION_SEEK = "com.moyue.app.audio.SEEK";
    static final String ACTION_STOP = "com.moyue.app.audio.STOP";
    static final String ACTION_TOGGLE = "com.moyue.app.audio.TOGGLE";
    static final String ACTION_PREV = "com.moyue.app.audio.PREV";
    static final String ACTION_NEXT = "com.moyue.app.audio.NEXT";
    static final String ACTION_METADATA = "com.moyue.app.audio.METADATA";

    static final String EXTRA_URL = "url";
    static final String EXTRA_POSITION = "position";
    static final String EXTRA_SECONDS = "seconds";
    static final String EXTRA_TITLE = "title";
    static final String EXTRA_SUBTITLE = "subtitle";

    /** JS 事件通道，由 AudioPlugin 注册/注销。 */
    public interface Emitter {
        void emit(JSObject event);
    }

    private static final Handler MAIN = new Handler(Looper.getMainLooper());
    private static volatile Emitter sEmitter;
    private static volatile boolean sRunning = false;
    private static volatile JSObject sState = new JSObject();

    public static void setEmitter(Emitter emitter) {
        sEmitter = emitter;
    }

    public static JSObject state() {
        return sState;
    }

    /** 服务已在前台运行时用普通 startService，避免 Android 12+ 的后台启动 FGS 限制。 */
    static void send(Context context, Intent intent) {
        try {
            if (sRunning) {
                context.startService(intent);
            } else {
                ContextCompat.startForegroundService(context, intent);
            }
        } catch (Exception e) {
            Log.e(TAG, "start audio service failed", e);
        }
    }

    private MediaPlayer mPlayer;
    private boolean mPrepared = false;
    private boolean mPlayWhenReady = false;
    private boolean mPlaying = false;
    private double mPendingSeek = 0;
    private double mDuration = 0;
    private String mUrl = "";
    private String mTitle = "";
    private String mSubtitle = "";

    private Handler mHandler;
    private AudioManager mAudioManager;
    private MediaSession mSession;
    private AudioFocusRequest mFocusRequest;
    private boolean mFocusGranted = false;
    private boolean mNoisyRegistered = false;
    private boolean mForegroundStarted = false;

    private final BroadcastReceiver mNoisyReceiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context context, Intent intent) {
            if (AudioManager.ACTION_AUDIO_BECOMING_NOISY.equals(intent.getAction()) && mPlaying) {
                pausePlayback();
            }
        }
    };

    private final AudioManager.OnAudioFocusChangeListener mFocusListener = focusChange -> {
        if ((focusChange == AudioManager.AUDIOFOCUS_LOSS
                || focusChange == AudioManager.AUDIOFOCUS_LOSS_TRANSIENT) && mPlaying) {
            pausePlayback();
        }
    };

    private final Runnable mTicker = new Runnable() {
        @Override
        public void run() {
            if (!mPlaying) return;
            emit("time", currentTime(), mDuration, null);
            publishPlaybackState();
            if (mHandler != null) mHandler.postDelayed(this, TICK_MS);
        }
    };

    @Override
    public void onCreate() {
        super.onCreate();
        sRunning = true;
        ensureInitialized();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        ensureInitialized();
        String action = intent == null ? null : intent.getAction();
        if (action == null) {
            stopEverything();
            return START_NOT_STICKY;
        }
        // startForegroundService 后必须在数秒内调用 startForeground()，即使是 STOP 指令
        // （否则系统判定「未进入前台」而终止服务）；stopEverything() 随后会移除通知。
        startAsForeground();
        handleAction(action, intent);
        return START_NOT_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void ensureInitialized() {
        if (mHandler == null) mHandler = new Handler(Looper.getMainLooper());
        if (mAudioManager == null) mAudioManager = (AudioManager) getSystemService(Context.AUDIO_SERVICE);
        createNotificationChannel();
        if (mSession == null) initMediaSession();
        if (!mNoisyRegistered) {
            IntentFilter filter = new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY);
            try {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    registerReceiver(mNoisyReceiver, filter, Context.RECEIVER_NOT_EXPORTED);
                } else {
                    registerReceiver(mNoisyReceiver, filter);
                }
                mNoisyRegistered = true;
            } catch (Exception e) {
                Log.w(TAG, "register noisy receiver failed", e);
            }
        }
    }

    private void handleAction(String action, Intent intent) {
        switch (action) {
            case ACTION_LOAD:
                load(intent.getStringExtra(EXTRA_URL),
                        intent.getDoubleExtra(EXTRA_POSITION, 0),
                        intent.getStringExtra(EXTRA_TITLE),
                        intent.getStringExtra(EXTRA_SUBTITLE));
                break;
            case ACTION_PLAY:
                startPlayback();
                break;
            case ACTION_PAUSE:
                pausePlayback();
                break;
            case ACTION_SEEK:
                seek(intent.getDoubleExtra(EXTRA_SECONDS, 0));
                break;
            case ACTION_METADATA:
                setMetadata(intent.getStringExtra(EXTRA_TITLE), intent.getStringExtra(EXTRA_SUBTITLE));
                break;
            case ACTION_TOGGLE:
                if (mPlaying) pausePlayback();
                else startPlayback();
                break;
            case ACTION_PREV:
                emit("prev", currentTime(), mDuration, null);
                break;
            case ACTION_NEXT:
                emit("next", currentTime(), mDuration, null);
                break;
            case ACTION_STOP:
                stopEverything();
                break;
            default:
                break;
        }
    }

    // ===== 播放 =====

    private void load(String url, double position, String title, String subtitle) {
        if (url == null || url.isEmpty()) return;
        mUrl = url;
        // setMetadata 通常由 JS 在 load 之后到达；这里不要用空值覆盖已同步的书名/段名
        if (title != null && !title.isEmpty()) mTitle = title;
        if (subtitle != null && !subtitle.isEmpty()) mSubtitle = subtitle;
        mPendingSeek = Math.max(0, position);
        releasePlayer();
        mPlayWhenReady = false;
        mPlaying = false;
        mDuration = 0;
        stopTicker();

        try {
            MediaPlayer player = new MediaPlayer();
            mPlayer = player;
            player.setAudioAttributes(audioAttributes());
            // 播放期间持有 CPU 唤醒锁，保证锁屏后 ended 事件仍能被 JS 接收并自动续播。
            player.setWakeMode(getApplicationContext(), PowerManager.PARTIAL_WAKE_LOCK);
            player.setDataSource(url);
            player.setOnPreparedListener(mp -> onPrepared());
            player.setOnCompletionListener(mp -> onCompletion());
            player.setOnErrorListener((mp, what, extra) -> {
                mPlaying = false;
                stopTicker();
                emit("error", 0, 0, "播放失败(" + what + "," + extra + ")，请检查服务器地址与音频文件");
                publishState();
                refreshNotification();
                return true;
            });
            player.prepareAsync();
        } catch (Exception e) {
            Log.e(TAG, "load audio failed", e);
            releasePlayer();
            emit("error", 0, 0, "音频加载失败：" + e.getMessage());
            publishState();
            return;
        }

        updateSessionMetadata();
        refreshNotification();
        publishState();
        emit("load", 0, 0, null);
    }

    private void onPrepared() {
        if (mPlayer == null) return;
        mPrepared = true;
        try {
            int ms = mPlayer.getDuration();
            if (ms > 0) mDuration = ms / 1000.0;
        } catch (Exception ignored) {}
        if (mPendingSeek > 0) {
            try { mPlayer.seekTo((int) (mPendingSeek * 1000)); } catch (Exception ignored) {}
            mPendingSeek = 0;
        }
        updateSessionMetadata();
        emit("ready", currentTime(), mDuration, null);
        publishState();
        if (mPlayWhenReady) startPlayback();
    }

    private void startPlayback() {
        if (mPlayer == null) return;
        if (!mPrepared) {
            mPlayWhenReady = true;
            return;
        }
        requestFocus();
        try {
            mPlayer.start();
        } catch (Exception e) {
            emit("error", 0, 0, "无法开始播放：" + e.getMessage());
            return;
        }
        mPlaying = true;
        mPlayWhenReady = false;
        startTicker();
        publishPlaybackState();
        refreshNotification();
        publishState();
        emit("play", currentTime(), mDuration, null);
    }

    private void pausePlayback() {
        if (mPlayer != null && mPrepared) {
            try { mPlayer.pause(); } catch (Exception ignored) {}
        }
        mPlaying = false;
        mPlayWhenReady = false;
        stopTicker();
        publishPlaybackState();
        refreshNotification();
        publishState();
        emit("pause", currentTime(), mDuration, null);
    }

    private void seek(double seconds) {
        double sec = Math.max(0, seconds);
        mPendingSeek = sec;
        if (mPlayer != null && mPrepared) {
            try {
                mPlayer.seekTo((int) (sec * 1000));
                mPendingSeek = 0;
            } catch (Exception ignored) {}
        }
        emit("time", sec, mDuration, null);
        publishState();
    }

    private void onCompletion() {
        mPlaying = false;
        mPlayWhenReady = false;
        stopTicker();
        publishPlaybackState();
        refreshNotification();
        publishState();
        // JS 收到 ended 后会设置下一段的 src；此处保持服务存活等待下一条指令。
        emit("ended", mDuration, mDuration, null);
    }

    private void setMetadata(String title, String subtitle) {
        mTitle = title == null ? "" : title;
        mSubtitle = subtitle == null ? "" : subtitle;
        updateSessionMetadata();
        refreshNotification();
    }

    private void stopEverything() {
        stopTicker();
        releasePlayer();
        mPlaying = false;
        mPlayWhenReady = false;
        mDuration = 0;
        mPendingSeek = 0;
        mUrl = "";
        if (mSession != null) {
            mSession.setPlaybackState(new PlaybackState.Builder()
                    .setState(PlaybackState.STATE_NONE, 0, 0).build());
        }
        abandonFocus();
        // 保留一个空服务（不持有播放器/唤醒锁），下次播放可立即复用，避免反复 startForegroundService。
        if (mForegroundStarted) {
            stopForeground(STOP_FOREGROUND_REMOVE);
            mForegroundStarted = false;
        }
        publishState();
        emit("stop", 0, 0, null);
    }

    private void releasePlayer() {
        if (mPlayer != null) {
            try { mPlayer.reset(); } catch (Exception ignored) {}
            try { mPlayer.release(); } catch (Exception ignored) {}
            mPlayer = null;
        }
        mPrepared = false;
    }

    private double currentTime() {
        if (mPlayer != null && mPrepared) {
            try { return Math.max(0, mPlayer.getCurrentPosition() / 1000.0); } catch (Exception ignored) {}
        }
        return 0;
    }

    private void startTicker() {
        if (mHandler == null) return;
        mHandler.removeCallbacks(mTicker);
        mHandler.postDelayed(mTicker, TICK_MS);
    }

    private void stopTicker() {
        if (mHandler != null) mHandler.removeCallbacks(mTicker);
    }

    // ===== 前台服务通知 / MediaSession =====

    private void createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel channel = new NotificationChannel(
                    CHANNEL_ID, getString(R.string.audio_channel_name), NotificationManager.IMPORTANCE_LOW);
            channel.setDescription(getString(R.string.audio_channel_desc));
            channel.setShowBadge(false);
            NotificationManager nm = getSystemService(NotificationManager.class);
            if (nm != null) nm.createNotificationChannel(channel);
        }
    }

    private void initMediaSession() {
        mSession = new MediaSession(this, "MoyueAudio");
        mSession.setFlags(MediaSession.FLAG_HANDLES_MEDIA_BUTTONS
                | MediaSession.FLAG_HANDLES_TRANSPORT_CONTROLS);
        mSession.setSessionActivity(PendingIntent.getActivity(this, 1,
                new Intent(this, MainActivity.class), pendingFlags()));
        mSession.setCallback(new MediaSession.Callback() {
            @Override public void onPlay() { startPlayback(); }
            @Override public void onPause() { pausePlayback(); }
            @Override public void onStop() { stopEverything(); }
            @Override public void onSkipToNext() { handleAction(ACTION_NEXT, null); }
            @Override public void onSkipToPrevious() { handleAction(ACTION_PREV, null); }
            @Override public void onSeekTo(long pos) { seek(pos / 1000.0); }
        });
        mSession.setActive(true);
        publishPlaybackState();
    }

    private void publishPlaybackState() {
        if (mSession == null) return;
        try {
            PlaybackState.Builder builder = new PlaybackState.Builder()
                    .setActions(PlaybackState.ACTION_PLAY
                            | PlaybackState.ACTION_PAUSE
                            | PlaybackState.ACTION_PLAY_PAUSE
                            | PlaybackState.ACTION_SKIP_TO_NEXT
                            | PlaybackState.ACTION_SKIP_TO_PREVIOUS
                            | PlaybackState.ACTION_SEEK_TO
                            | PlaybackState.ACTION_STOP)
                    .setState(mPlaying ? PlaybackState.STATE_PLAYING : PlaybackState.STATE_PAUSED,
                            (long) (currentTime() * 1000), mPlaying ? 1.0f : 0f);
            mSession.setPlaybackState(builder.build());
        } catch (Exception e) {
            Log.w(TAG, "set playback state failed", e);
        }
    }

    private void updateSessionMetadata() {
        if (mSession == null) return;
        MediaMetadata.Builder builder = new MediaMetadata.Builder()
                .putString(MediaMetadata.METADATA_KEY_TITLE, mTitle.isEmpty() ? "有声书" : mTitle)
                .putString(MediaMetadata.METADATA_KEY_ARTIST, mSubtitle.isEmpty() ? "墨阅" : mSubtitle)
                .putString(MediaMetadata.METADATA_KEY_ALBUM, "墨阅 · 有声书");
        if (mDuration > 0) builder.putLong(MediaMetadata.METADATA_KEY_DURATION, (long) (mDuration * 1000));
        mSession.setMetadata(builder.build());
    }

    private void startAsForeground() {
        if (mForegroundStarted) return;
        mForegroundStarted = true;
        try {
            ServiceCompat.startForeground(this, NOTIFICATION_ID, buildNotification(),
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK);
        } catch (Exception e) {
            // 未授予 POST_NOTIFICATIONS 时通知不会展示，但前台服务本身仍可运行，播放不受影响。
            Log.w(TAG, "startForeground failed", e);
        }
    }

    private Notification buildNotification() {
        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(this, CHANNEL_ID)
                : new Notification.Builder(this);
        builder.setSmallIcon(R.drawable.ic_stat_audio)
                .setContentTitle(mTitle.isEmpty() ? "有声书" : mTitle)
                .setContentText(mSubtitle.isEmpty() ? "墨阅" : mSubtitle)
                .setContentIntent(PendingIntent.getActivity(this, 1,
                        new Intent(this, MainActivity.class), pendingFlags()))
                .setOnlyAlertOnce(true)
                .setOngoing(mPlaying)
                .setShowWhen(false)
                .setVisibility(Notification.VISIBILITY_PUBLIC);
        builder.addAction(new Notification.Action.Builder(android.R.drawable.ic_media_previous,
                "上一段", serviceIntent(ACTION_PREV, 2)).build());
        builder.addAction(new Notification.Action.Builder(
                mPlaying ? android.R.drawable.ic_media_pause : android.R.drawable.ic_media_play,
                mPlaying ? "暂停" : "播放", serviceIntent(ACTION_TOGGLE, 3)).build());
        builder.addAction(new Notification.Action.Builder(android.R.drawable.ic_media_next,
                "下一段", serviceIntent(ACTION_NEXT, 4)).build());
        Notification.MediaStyle style = new Notification.MediaStyle()
                .setShowActionsInCompactView(0, 1, 2);
        if (mSession != null) style.setMediaSession(mSession.getSessionToken());
        builder.setStyle(style);
        return builder.build();
    }

    private void refreshNotification() {
        if (!mForegroundStarted) return;
        try {
            NotificationManager nm = (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
            if (nm != null) nm.notify(NOTIFICATION_ID, buildNotification());
        } catch (Exception e) {
            Log.w(TAG, "refresh notification failed", e);
        }
    }

    private PendingIntent serviceIntent(String action, int requestCode) {
        Intent intent = new Intent(this, AudioPlayerService.class).setAction(action);
        return PendingIntent.getService(this, requestCode, intent, pendingFlags());
    }

    private static int pendingFlags() {
        int flags = PendingIntent.FLAG_UPDATE_CURRENT;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) flags |= PendingIntent.FLAG_IMMUTABLE;
        return flags;
    }

    // ===== 音频焦点 =====

    private AudioAttributes audioAttributes() {
        return new AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_MEDIA)
                .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC)
                .build();
    }

    private void requestFocus() {
        if (mFocusGranted || mAudioManager == null) return;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            if (mFocusRequest == null) {
                mFocusRequest = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN)
                        .setAudioAttributes(audioAttributes())
                        .setWillPauseWhenDucked(true)
                        .setOnAudioFocusChangeListener(mFocusListener, mHandler)
                        .build();
            }
            mFocusGranted = mAudioManager.requestAudioFocus(mFocusRequest)
                    == AudioManager.AUDIOFOCUS_REQUEST_GRANTED;
        } else {
            mFocusGranted = mAudioManager.requestAudioFocus(mFocusListener,
                    AudioManager.STREAM_MUSIC, AudioManager.AUDIOFOCUS_GAIN)
                    == AudioManager.AUDIOFOCUS_REQUEST_GRANTED;
        }
    }

    private void abandonFocus() {
        if (mAudioManager == null) return;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            if (mFocusRequest != null) mAudioManager.abandonAudioFocusRequest(mFocusRequest);
        } else {
            mAudioManager.abandonAudioFocus(mFocusListener);
        }
        mFocusGranted = false;
    }

    // ===== 事件 / 状态 =====

    private void emit(String type, double currentTime, double duration, String message) {
        JSObject event = new JSObject();
        event.put("type", type)
                .put("currentTime", currentTime)
                .put("duration", duration);
        if (message != null) event.put("message", message);
        MAIN.post(() -> {
            Emitter emitter = sEmitter;
            if (emitter != null) emitter.emit(event);
        });
    }

    private void publishState() {
        JSObject snapshot = new JSObject();
        snapshot.put("url", mUrl)
                .put("title", mTitle)
                .put("subtitle", mSubtitle)
                .put("playing", mPlaying)
                .put("prepared", mPrepared)
                .put("currentTime", currentTime())
                .put("duration", mDuration);
        sState = snapshot;
    }

    @Override
    public void onDestroy() {
        super.onDestroy();
        stopTicker();
        releasePlayer();
        abandonFocus();
        if (mNoisyRegistered) {
            try { unregisterReceiver(mNoisyReceiver); } catch (Exception ignored) {}
            mNoisyRegistered = false;
        }
        if (mSession != null) {
            mSession.setActive(false);
            mSession.release();
            mSession = null;
        }
        sRunning = false;
    }
}
