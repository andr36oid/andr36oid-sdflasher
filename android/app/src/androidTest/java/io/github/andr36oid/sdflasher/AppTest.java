package io.github.andr36oid.sdflasher;

import static org.junit.Assert.*;

import android.content.Context;
import android.content.res.Configuration;
import android.os.Build;
import android.view.*;
import android.widget.TextView;
import androidx.test.core.app.ActivityScenario;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import io.github.andr36oid.mobile.*;
import java.io.File;
import java.util.Locale;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import org.json.JSONObject;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;

@RunWith(AndroidJUnit4.class)
public class AppTest {
  private Context context;

  @Before
  public void prepare() throws Exception {
    context = InstrumentationRegistry.getInstrumentation().getTargetContext();
    context.getSharedPreferences("settings", 0).edit().clear().commit();
    if (Build.VERSION.SDK_INT >= 33) {
      InstrumentationRegistry.getInstrumentation()
          .getUiAutomation()
          .executeShellCommand(
              "pm grant " + context.getPackageName() + " android.permission.POST_NOTIFICATIONS")
          .close();
    }
  }

  private boolean contains(View view, String text) {
    if (view instanceof TextView && ((TextView) view).getText().toString().equals(text))
      return true;
    if (view instanceof ViewGroup) {
      ViewGroup group = (ViewGroup) view;
      for (int n = 0; n < group.getChildCount(); n++)
        if (contains(group.getChildAt(n), text)) return true;
    }
    return false;
  }

  @Test
  public void languageChoiceSurvivesRecreation() {
    try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
      for (String code : I18n.CODES) {
        context.getSharedPreferences("settings", 0).edit().putString("language", code).commit();
        scenario.recreate();
        scenario.onActivity(
            activity -> {
              assertEquals(code, I18n.code(activity));
              assertTrue(
                  contains(
                      activity.getWindow().getDecorView(),
                      Mobile.translate(code, "Set up or update your andr36oid card")));
              assertTrue(contains(activity.getWindow().getDecorView(), I18n.name(activity)));
            });
      }
    }
  }

  @Test
  public void automaticLocaleUsesBrazilianPortugueseAndEnglishFallback() {
    Configuration config = new Configuration(context.getResources().getConfiguration());
    config.setLocale(new Locale("pt", "BR"));
    assertEquals("pt-BR", I18n.code(context.createConfigurationContext(config)));
    config.setLocale(Locale.FRENCH);
    assertEquals("en", I18n.code(context.createConfigurationContext(config)));
    config.setLocale(Locale.SIMPLIFIED_CHINESE);
    assertEquals("zh-Hans", I18n.code(context.createConfigurationContext(config)));
  }

  static class MemoryDisk implements Disk {
    int writes, reads;

    public long capacity() {
      return 1L << 30;
    }

    public String identity() {
      return "emulated-usb-reader";
    }

    public byte[] read(long offset, long length) {
      reads++;
      assertEquals(0, offset % 512);
      assertEquals(0, length % 512);
      return new byte[(int) length];
    }

    public void write(long offset, byte[] data) {
      writes++;
    }

    public void flush() {}
  }

  @Test
  public void nativeBridgeInspectsWithoutWritingAndRejectsMissingImage() throws Exception {
    MemoryDisk disk = new MemoryDisk();
    JSONObject card = new JSONObject(Mobile.inspectDisk(disk));
    assertFalse(card.getBoolean("installed"));
    assertTrue(disk.reads > 0);
    Session session =
        Mobile.newSession(new File(context.getFilesDir(), "test-engine").getAbsolutePath());
    try {
      session.write(disk, "{}", null);
      fail("write without source succeeded");
    } catch (Exception expected) {
      assertTrue(expected.getMessage().contains("Treble"));
    }
    assertEquals(0, disk.writes);
  }

  @Test
  public void foregroundWorkSurvivesActivityRecreation() throws Exception {
    CountDownLatch started = new CountDownLatch(1), finish = new CountDownLatch(1);
    try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
      scenario.onActivity(
          activity ->
              WorkState.start(
                  activity.getApplicationContext(),
                  false,
                  () -> {
                    started.countDown();
                    finish.await(30, TimeUnit.SECONDS);
                  }));
      assertTrue(started.await(10, TimeUnit.SECONDS));
      scenario.recreate();
      scenario.onActivity(activity -> assertTrue(WorkState.busy));
    } finally {
      finish.countDown();
    }
    long limit = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
    while (WorkState.busy && System.nanoTime() < limit) Thread.sleep(50);
    assertFalse(WorkState.busy);
  }
}
