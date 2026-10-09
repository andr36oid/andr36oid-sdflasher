package io.github.andr36oid.sdflasher;

import android.content.Context;
import android.content.Intent;
import android.os.Build;
import io.github.andr36oid.bindings.mobile.Mobile;
import io.github.andr36oid.bindings.mobile.Session;
import java.io.File;
import java.util.List;
import org.json.JSONObject;

final class WorkState {
  interface Work {
    void run() throws Exception;
  }

  static volatile Session engine;
  static volatile JSONObject appUpdate;
  static boolean updateCheckStarted, updateShown;
  static volatile JSONObject image, card, progress, plan;
  static volatile org.json.JSONArray releases;
  static volatile String reviewOptions = "";
  static volatile boolean reviewReady = false;
  static volatile UsbCard selected;
  static volatile List<UsbCard> discovered;
  static volatile String identity = "",
      profile = "",
      custom = "",
      error = "",
      message = "",
      result = "";
  static volatile boolean customAccepted = false, update = false, noROMs = false, busy = false;
  static volatile long revision = 0;
  static Work pending;
  static boolean usbJob;

  static synchronized void checkAppUpdate() {
    if (updateCheckStarted) return;
    updateCheckStarted = true;
    new Thread(
            () -> {
              try {
                String value = Mobile.checkAppUpdate(BuildConfig.VERSION_NAME);
                if (!value.isEmpty()) appUpdate = new JSONObject(value);
              } catch (Exception ignored) {
              }
            },
            "app-update-check")
        .start();
  }

  static synchronized void init(Context context) throws Exception {
    if (engine == null)
      engine =
          Mobile.newSession(
              new File(context.getFilesDir(), "images-and-recovery").getAbsolutePath());
  }

  static synchronized void start(Context context, boolean usb, Work work) {
    if (busy) return;
    busy = true;
    error = "";
    message = "Working…";
    result = "";
    progress = null;
    plan = null;
    pending = work;
    usbJob = usb;
    revision++;
    Intent intent = new Intent(context, FlashService.class);
    try {
      if (Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent);
      else context.startService(intent);
    } catch (RuntimeException e) {
      busy = false;
      pending = null;
      error = e.getMessage();
      revision++;
    }
  }

  static void event(String event) {
    try {
      progress = new JSONObject(event);
      if (progress.has("plan")) plan = progress.getJSONObject("plan");
      revision++;
    } catch (Exception ignored) {
    }
  }

  static JSONObject options() throws Exception {
    return new JSONObject()
        .put("identity", identity)
        .put("fingerprint", card.getString("fingerprint"))
        .put("mode", update ? "update" : "install")
        .put("profile", profile)
        .put("no_roms", noROMs)
        .put("custom", custom)
        .put("custom_accepted", customAccepted);
  }
}
