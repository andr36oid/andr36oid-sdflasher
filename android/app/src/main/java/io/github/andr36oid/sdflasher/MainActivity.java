package io.github.andr36oid.sdflasher;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.database.Cursor;
import android.hardware.usb.*;
import android.net.Uri;
import android.os.*;
import android.provider.OpenableColumns;
import android.view.*;
import android.widget.*;
import io.github.andr36oid.mobile.Mobile;
import java.io.*;
import java.util.*;
import org.json.*;

public final class MainActivity extends Activity {
  private final Handler handler = new Handler(Looper.getMainLooper());
  private long rendered = -1;
  private LinearLayout body;
  private TextView imageLabel, cardLabel, status, customLabel;
  private Button language, console, profile, chooseCard, action;
  private final List<View> controls = new ArrayList<>();
  private CheckBox noROMs;
  private RadioButton install, update;
  private ProgressBar progress;
  private DiskMap map;
  private UsbCard pendingReader;
  private String lastError = "", lastResult = "";

  private String t(String key, Object... args) {
    return I18n.text(this, key, args);
  }

  private final Runnable poll =
      new Runnable() {
        public void run() {
          if (WorkState.revision != rendered) {
            rendered = WorkState.revision;
            render();
          }
          handler.postDelayed(this, 200);
        }
      };
  private final BroadcastReceiver usbEvents =
      new BroadcastReceiver() {
        @Override
        public void onReceive(Context context, Intent intent) {
          if ((getPackageName() + ".USB_PERMISSION").equals(intent.getAction())) {
            UsbDevice device = intent.getParcelableExtra(UsbManager.EXTRA_DEVICE);
            if (pendingReader != null
                && device != null
                && device.getDeviceId() == pendingReader.device.getDeviceId()) {
              if (intent.getBooleanExtra(UsbManager.EXTRA_PERMISSION_GRANTED, false))
                discover(pendingReader);
              else showError(t("USB permission was denied."));
            }
          } else if (UsbManager.ACTION_USB_DEVICE_DETACHED.equals(intent.getAction())) {
            UsbDevice device = intent.getParcelableExtra(UsbManager.EXTRA_DEVICE);
            if (device != null
                && WorkState.selected != null
                && device.getDeviceId() == WorkState.selected.device.getDeviceId()) {
              WorkState.selected = null;
              WorkState.card = null;
              WorkState.identity = "";
              WorkState.revision++;
            }
          }
        }
      };

  @Override
  public void onCreate(Bundle state) {
    super.onCreate(state);
    try {
      WorkState.init(getApplicationContext());
    } catch (Exception e) {
      showError(e.toString());
      return;
    }
    IntentFilter filter = new IntentFilter(getPackageName() + ".USB_PERMISSION");
    filter.addAction(UsbManager.ACTION_USB_DEVICE_DETACHED);
    if (Build.VERSION.SDK_INT >= 33)
      registerReceiver(usbEvents, filter, Context.RECEIVER_NOT_EXPORTED);
    else registerReceiver(usbEvents, filter);
    build();
    if (Build.VERSION.SDK_INT >= 33
        && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)
            != android.content.pm.PackageManager.PERMISSION_GRANTED)
      requestPermissions(new String[] {Manifest.permission.POST_NOTIFICATIONS}, 3);
  }

  @Override
  protected void onResume() {
    super.onResume();
    rendered = -1;
    handler.post(poll);
  }

  @Override
  protected void onPause() {
    handler.removeCallbacks(poll);
    super.onPause();
  }

  @Override
  protected void onDestroy() {
    try {
      unregisterReceiver(usbEvents);
    } catch (IllegalArgumentException ignored) {
    }
    super.onDestroy();
  }

  private int dp(int n) {
    return Math.round(n * getResources().getDisplayMetrics().density);
  }

  private TextView label(String text, boolean heading) {
    TextView view = new TextView(this);
    view.setText(text);
    view.setTextSize(heading ? 19 : 15);
    view.setTextColor(0xff18304a);
    if (heading) view.setTypeface(null, android.graphics.Typeface.BOLD);
    view.setPadding(0, dp(9), 0, dp(9));
    body.addView(view);
    return view;
  }

  private Button button(String key, View.OnClickListener action) {
    Button view = new Button(this);
    view.setText(t(key));
    view.setAllCaps(false);
    view.setOnClickListener(action);
    body.addView(view);
    controls.add(view);
    return view;
  }

  private void build() {
    controls.clear();
    ScrollView scroll = new ScrollView(this);
    scroll.setFillViewport(true);
    body = new LinearLayout(this);
    body.setOrientation(LinearLayout.VERTICAL);
    body.setPadding(dp(18), dp(12), dp(18), dp(24));
    body.setBackgroundColor(0xfff8fafd);
    scroll.addView(body);
    setContentView(scroll);
    scroll.setOnApplyWindowInsetsListener(
        (view, insets) -> {
          view.setPadding(
              insets.getSystemWindowInsetLeft(),
              insets.getSystemWindowInsetTop(),
              insets.getSystemWindowInsetRight(),
              insets.getSystemWindowInsetBottom());
          return insets;
        });
    label("andr36oid SD Flasher", true);
    language = new Button(this);
    language.setAllCaps(false);
    language.setText(I18n.name(this));
    language.setOnClickListener(v -> languages());
    body.addView(language);
    label(t("Set up or update your andr36oid card"), true);
    label(
        t("Connect a USB card reader directly to your Android device. No root is needed."), false);
    label(t("1. Release"), true);
    button("Download a release…", v -> releases());
    button("Choose image from disk…", v -> pick(1));
    imageLabel = label("", false);
    label(t("2. Console and screen"), true);
    console = button("Choose your console / board family", v -> chooseConsole());
    profile = button("Choose your board and screen", v -> chooseProfile());
    button(
        "I don’t know my board or panel",
        v ->
            info(
                "Finding your board and panel",
                "Use your console’s board markings and screen information. Similar-looking consoles"
                    + " can require different DTBs."));
    button(
        "Advanced: custom DTB",
        v ->
            new AlertDialog.Builder(this)
                .setTitle(t("Custom Android DTB"))
                .setMessage(t(Mobile.customWarning()))
                .setPositiveButton(t("Choose custom Android DTB…"), (d, w) -> pick(2))
                .setNegativeButton(
                    t("Use shipped DTB"),
                    (d, w) -> {
                      WorkState.custom = "";
                      WorkState.customAccepted = false;
                      WorkState.revision++;
                    })
                .show());
    customLabel = label("", false);
    label(t("3. microSD card"), true);
    chooseCard = button("Select a microSD card", v -> readers());
    cardLabel = label("", false);
    RadioGroup modes = new RadioGroup(this);
    install = new RadioButton(this);
    update = new RadioButton(this);
    install.setId(View.generateViewId());
    update.setId(View.generateViewId());
    install.setText(t("Install a new card (erase)"));
    update.setText(t("Update an existing installation"));
    modes.addView(install);
    modes.addView(update);
    body.addView(modes);
    controls.add(install);
    controls.add(update);
    modes.setOnCheckedChangeListener(
        (g, id) -> {
          WorkState.update = id == update.getId();
          if (WorkState.update && WorkState.card != null)
            WorkState.noROMs = WorkState.card.optBoolean("no_roms");
          WorkState.revision++;
        });
    noROMs = new CheckBox(this);
    noROMs.setText(t("Use all available storage for Android"));
    body.addView(noROMs);
    noROMs.setOnCheckedChangeListener((b, checked) -> WorkState.noROMs = checked);
    label(
        t(
            "Unchecked: reserve 16 GiB for Android and use the remaining space for a games"
                + " partition readable on your computer. Updates keep the existing arrangement."),
        false);
    action = button("Continue", v -> review());
    progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
    progress.setMax(1000);
    body.addView(progress);
    status = label("", false);
    map = new DiskMap(this);
    body.addView(map, new LinearLayout.LayoutParams(-1, dp(160)));
    label(t("Pending · Writing · Written · Verified · Preserved"), false);
    button("Restore an interrupted update…", v -> recover());
    button("About and licenses", v -> about());
    label(BuildConfig.VERSION_NAME + " · GPLv3", false);
    rendered = -1;
    render();
  }

  private void languages() {
    String[] labels = new String[I18n.NAMES.length + 1];
    labels[0] = t("System language");
    System.arraycopy(I18n.NAMES, 0, labels, 1, I18n.NAMES.length);
    new AlertDialog.Builder(this)
        .setTitle(t("Language"))
        .setItems(
            labels,
            (d, n) -> {
              getSharedPreferences("settings", 0)
                  .edit()
                  .putString("language", n == 0 ? "" : I18n.CODES[n - 1])
                  .apply();
              build();
            })
        .show();
  }

  private void render() {
    if (imageLabel == null) return;
    boolean busy = WorkState.busy;
    if (WorkState.error.isEmpty()) lastError = "";
    if (WorkState.result.isEmpty()) lastResult = "";
    for (View view : controls) view.setEnabled(!busy);
    imageLabel.setText(
        WorkState.image == null
            ? t("Choose a release to see its supported consoles and panels.")
            : WorkState.image.optString("version"));
    customLabel.setText(
        WorkState.custom.isEmpty()
            ? t("Use the DTB shipped with the selected profile.")
            : t("Custom Android DTB") + ": " + new File(WorkState.custom).getName());
    JSONObject selected = selectedProfile();
    console.setText(
        selected == null ? t("Choose your console / board family") : selected.optString("console"));
    profile.setText(
        selected == null ? t("Choose your board and screen") : selected.optString("name"));
    if (WorkState.selected != null) chooseCard.setText(WorkState.selected.label());
    else chooseCard.setText(t("Select a microSD card"));
    boolean installed = WorkState.card != null && WorkState.card.optBoolean("installed");
    cardLabel.setText(
        WorkState.card == null
            ? t("Insert a microSD card, then select it below.")
            : installed
                ? t(
                    WorkState.card.optBoolean("treble")
                        ? "Treble installation"
                        : "Legacy andr36oid — will migrate to Treble")
                : t("New installation required"));
    install.setChecked(!WorkState.update);
    update.setChecked(WorkState.update);
    update.setEnabled(!busy && installed);
    noROMs.setChecked(WorkState.noROMs);
    noROMs.setEnabled(!busy && !WorkState.update);
    action.setEnabled(
        !busy && WorkState.image != null && WorkState.card != null && !WorkState.profile.isEmpty());
    JSONObject p = WorkState.progress;
    String phase = p == null ? "" : p.optString("phase");
    status.setText(
        WorkState.error.isEmpty()
            ? t(busy ? (phase.isEmpty() ? "Working…" : phase) : WorkState.message)
            : t("Operation could not finish") + "\n" + WorkState.error);
    progress.setIndeterminate(busy && (p == null || p.optLong("total") == 0));
    if (p != null && p.optLong("total") > 0)
      progress.setProgress((int) (1000.0 * p.optLong("done") / p.optLong("total")));
    map.update(WorkState.plan, p);
    if (!busy && WorkState.discovered != null) {
      List<UsbCard> cards = WorkState.discovered;
      WorkState.discovered = null;
      chooseDiscovered(cards);
    }
    if (!busy && WorkState.reviewReady) {
      WorkState.reviewReady = false;
      if (WorkState.error.isEmpty()) confirmWrite();
    }
    if (!busy && WorkState.releases != null) {
      JSONArray assets = WorkState.releases;
      WorkState.releases = null;
      showReleases(assets);
    }
    if (!busy && !WorkState.error.isEmpty() && !WorkState.error.equals(lastError)) {
      lastError = WorkState.error;
      showError(WorkState.error);
    }
    if (!busy && !WorkState.result.isEmpty() && !WorkState.result.equals(lastResult)) {
      lastResult = WorkState.result;
      info("Written and verified", WorkState.message);
    }
  }

  private JSONObject selectedProfile() {
    JSONArray ps = WorkState.image == null ? null : WorkState.image.optJSONArray("profiles");
    if (ps != null)
      for (int n = 0; n < ps.length(); n++) {
        JSONObject p = ps.optJSONObject(n);
        if (p != null && p.optString("id").equals(WorkState.profile)) return p;
      }
    return null;
  }

  private void chooseConsole() {
    if (WorkState.image == null) {
      info("Choose a release", "Choose a release to see its supported consoles and panels.");
      return;
    }
    TreeSet<String> names = new TreeSet<>();
    JSONArray ps = WorkState.image.optJSONArray("profiles");
    for (int n = 0; n < ps.length(); n++) names.add(ps.optJSONObject(n).optString("console"));
    String[] labels = names.toArray(new String[0]);
    new AlertDialog.Builder(this)
        .setTitle(t("Choose your console / board family"))
        .setItems(labels, (d, n) -> profilesFor(labels[n]))
        .show();
  }

  private void chooseProfile() {
    JSONObject selected = selectedProfile();
    if (selected == null) chooseConsole();
    else profilesFor(selected.optString("console"));
  }

  private void profilesFor(String family) {
    List<JSONObject> profiles = new ArrayList<>();
    List<String> names = new ArrayList<>();
    JSONArray ps = WorkState.image.optJSONArray("profiles");
    for (int n = 0; n < ps.length(); n++) {
      JSONObject p = ps.optJSONObject(n);
      if (p.optString("console").equals(family)) {
        profiles.add(p);
        names.add(
            p.optString("name")
                + (p.optBoolean("experimental") ? " (" + t("experimental") + ")" : ""));
      }
    }
    new AlertDialog.Builder(this)
        .setTitle(t("Choose your board and screen"))
        .setItems(
            names.toArray(new String[0]),
            (d, n) -> {
              WorkState.profile = profiles.get(n).optString("id");
              WorkState.revision++;
            })
        .show();
  }

  private void autoProfile() {
    if (WorkState.card != null && WorkState.image != null) {
      String p = WorkState.card.optString("profile");
      JSONArray ps = WorkState.image.optJSONArray("profiles");
      for (int n = 0; n < ps.length(); n++)
        if (ps.optJSONObject(n).optString("id").equals(p)) {
          WorkState.profile = p;
          break;
        }
    }
  }

  private void pick(int code) {
    Intent intent =
        new Intent(Intent.ACTION_OPEN_DOCUMENT)
            .setType("*/*")
            .addCategory(Intent.CATEGORY_OPENABLE);
    startActivityForResult(intent, code);
  }

  @Override
  protected void onActivityResult(int request, int result, Intent data) {
    super.onActivityResult(request, result, data);
    if (result != RESULT_OK || data == null || data.getData() == null) return;
    Uri uri = data.getData();
    String name = "";
    try (Cursor cursor =
        getContentResolver()
            .query(uri, new String[] {OpenableColumns.DISPLAY_NAME}, null, null, null)) {
      if (cursor != null && cursor.moveToFirst()) name = cursor.getString(0);
    }
    String lower = name.toLowerCase(Locale.ROOT);
    if (request == 1 && !(lower.endsWith(".img") || lower.endsWith(".zip"))) {
      showError(t("Choose an .img or .img.zip release image."));
      return;
    }
    final String extension = request == 2 ? ".dtb" : lower.endsWith(".zip") ? ".zip" : ".img";
    final Context app = getApplicationContext();
    WorkState.start(
        app,
        false,
        () -> {
          File file = File.createTempFile("import-", extension, app.getFilesDir());
          boolean keep = false;
          try (InputStream in = app.getContentResolver().openInputStream(uri);
              FileOutputStream out = new FileOutputStream(file)) {
            if (in == null) throw new IOException("Could not open selected file");
            byte[] buffer = new byte[1 << 20];
            long total = 0;
            int n;
            while ((n = in.read(buffer)) != -1) {
              total += n;
              if (total > (request == 2 ? 4L << 20 : 32L << 30))
                throw new IOException("Selected file exceeds the size limit");
              out.write(buffer, 0, n);
            }
            out.getFD().sync();
            if (request == 2) {
              WorkState.custom = file.getAbsolutePath();
              WorkState.customAccepted = true;
            } else {
              WorkState.image = null;
              WorkState.image =
                  new JSONObject(WorkState.engine.load(file.getAbsolutePath(), WorkState::event));
              WorkState.profile = "";
              autoProfile();
            }
            keep = true;
            WorkState.message = "Ready";
          } finally {
            if (!keep) file.delete();
          }
        });
  }

  private void releases() {
    WorkState.start(
        getApplicationContext(),
        false,
        () -> {
          WorkState.releases = new JSONArray(Mobile.releases());
          WorkState.message = "Ready";
        });
  }

  private void showReleases(JSONArray assets) {
    if (assets.length() == 0) {
      info(
          "No image downloads available",
          "No disk images are attached to andr36oid/releases yet. You can choose an image from"
              + " disk.");
      return;
    }
    String[] labels = new String[assets.length()];
    for (int n = 0; n < labels.length; n++) labels[n] = assets.optJSONObject(n).optString("name");
    Context app = getApplicationContext();
    new AlertDialog.Builder(this)
        .setTitle(t("Download a release"))
        .setItems(
            labels,
            (d, n) -> {
              WorkState.start(
                  app,
                  false,
                  () -> {
                    WorkState.image = null;
                    WorkState.image =
                        new JSONObject(
                            WorkState.engine.download(
                                assets.getJSONObject(n).toString(), WorkState::event));
                    WorkState.profile = "";
                    autoProfile();
                    WorkState.message = "Ready";
                  });
            })
        .show();
  }

  private void readers() {
    List<UsbCard> readers = UsbCard.readers(this);
    if (readers.isEmpty()) {
      info(
          "No removable cards found",
          "Connect a USB SD-card adapter directly. Internal SD slots, USB hubs, hard drives, and"
              + " SSDs are not supported.");
      return;
    }
    String[] labels = new String[readers.size()];
    for (int n = 0; n < labels.length; n++) labels[n] = readers.get(n).label();
    new AlertDialog.Builder(this)
        .setTitle(t("Select a microSD card"))
        .setItems(
            labels,
            (d, n) -> {
              pendingReader = readers.get(n);
              UsbManager manager = (UsbManager) getSystemService(USB_SERVICE);
              if (manager.hasPermission(pendingReader.device)) discover(pendingReader);
              else {
                Intent permission =
                    new Intent(getPackageName() + ".USB_PERMISSION").setPackage(getPackageName());
                manager.requestPermission(
                    pendingReader.device,
                    PendingIntent.getBroadcast(
                        this,
                        0,
                        permission,
                        PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT));
              }
            })
        .show();
  }

  private void discover(UsbCard reader) {
    Context app = getApplicationContext();
    WorkState.start(
        app,
        true,
        () -> {
          WorkState.discovered = reader.discover(app);
          WorkState.message = "Ready";
        });
  }

  private void chooseDiscovered(List<UsbCard> cards) {
    if (cards.isEmpty()) {
      info(
          "No removable cards found", "Insert a card with 512-byte logical sectors and try again.");
      return;
    }
    if (cards.size() == 1) {
      inspect(cards.get(0));
      return;
    }
    String[] labels = new String[cards.size()];
    for (int n = 0; n < labels.length; n++) labels[n] = cards.get(n).label();
    new AlertDialog.Builder(this)
        .setTitle(t("Select a microSD card"))
        .setItems(labels, (d, n) -> inspect(cards.get(n)))
        .show();
  }

  private void inspect(UsbCard target) {
    Context app = getApplicationContext();
    WorkState.start(
        app,
        true,
        () -> {
          try (UsbCard.Connection disk = target.open(app)) {
            WorkState.card = new JSONObject(Mobile.inspectDisk(disk));
            WorkState.identity = disk.identity();
            WorkState.selected = target;
            WorkState.update = WorkState.card.optBoolean("installed");
            WorkState.noROMs = WorkState.card.optBoolean("no_roms");
            autoProfile();
            WorkState.message = "Ready";
          }
        });
  }

  private void review() {
    try {
      WorkState.reviewOptions = WorkState.options().toString();
      UsbCard target = WorkState.selected;
      Context app = getApplicationContext();
      WorkState.reviewReady = false;
      WorkState.start(
          app,
          true,
          () -> {
            try (UsbCard.Connection disk = target.open(app)) {
              WorkState.plan = new JSONObject(WorkState.engine.plan(disk, WorkState.reviewOptions));
              WorkState.reviewReady = true;
            }
          });
    } catch (Exception e) {
      WorkState.reviewReady = false;
      showError(e.toString());
    }
  }

  private void confirmWrite() {
    UsbCard target = WorkState.selected;
    if (target == null) return;
    boolean updating = WorkState.update;
    String input = WorkState.reviewOptions;
    Context app = getApplicationContext();
    String message =
        t(
            updating
                ? "BOOT, system, and vendor will be updated together. The dedicated cache will be"
                    + " cleared. Userdata and games stay in place. A recovery backup will be"
                    + " saved before writing."
                : "Everything currently on this card will be erased.");
    if (!WorkState.custom.isEmpty()) message += "\n\n" + t(Mobile.customWarning());
    new AlertDialog.Builder(this)
        .setTitle(t(updating ? "Update this card?" : "Erase this card?"))
        .setMessage(target.label() + "\n\n" + message)
        .setNegativeButton(t("Cancel"), null)
        .setPositiveButton(
            t(updating ? "Update" : "Erase and install"),
            (dialog, which) -> {
              WorkState.start(
                  app,
                  true,
                  () -> {
                    try (UsbCard.Connection disk = target.open(app)) {
                      WorkState.result = WorkState.engine.write(disk, input, WorkState::event);
                      WorkState.message =
                          updating
                              ? "Your card has been written and verified. Apps, settings, saves,"
                                  + " and games were preserved."
                              : "Your card has been written and verified. Insert it into the"
                                  + " console and keep it powered on until Android setup"
                                  + " appears.";
                      WorkState.card = null;
                    }
                  });
            })
        .show();
  }

  private void recover() {
    if (WorkState.selected == null) {
      info("Choose a card", "Select the card that was being updated first.");
      return;
    }
    try {
      JSONArray backups = new JSONArray(WorkState.engine.recoveries());
      if (backups.length() == 0) {
        info("Recovery backups", "No recovery backups are available.");
        return;
      }
      String[] labels = new String[backups.length()];
      for (int n = 0; n < labels.length; n++) {
        JSONObject b = backups.getJSONObject(n);
        labels[n] = b.optString("created") + " · " + b.optString("version");
      }
      new AlertDialog.Builder(this)
          .setTitle(t("Recovery backups"))
          .setItems(
              labels,
              (d, n) -> {
                UsbCard target = WorkState.selected;
                Context app = getApplicationContext();
                String dir = backups.optJSONObject(n).optString("directory");
                new AlertDialog.Builder(this)
                    .setTitle(t("Restore previous installation"))
                    .setMessage(
                        t(
                                "Restore the saved operating system? Userdata and games remain"
                                    + " protected.")
                            + "\n\n"
                            + target.label())
                    .setNegativeButton(t("Cancel"), null)
                    .setPositiveButton(
                        t("Restore"),
                        (dialog, which) ->
                            WorkState.start(
                                app,
                                true,
                                () -> {
                                  try (UsbCard.Connection disk = target.open(app)) {
                                    WorkState.engine.restore(disk, dir, WorkState::event);
                                    WorkState.message =
                                        "The previous installation was restored and verified.";
                                    WorkState.result = dir;
                                    WorkState.card = null;
                                  }
                                }))
                    .show();
              })
          .show();
    } catch (Exception e) {
      showError(e.toString());
    }
  }

  private void about() {
    new AlertDialog.Builder(this)
        .setTitle(t("About and licenses"))
        .setMessage(
            "andr36oid SD Flasher\n"
                + BuildConfig.VERSION_NAME
                + "\nGPLv3\n\n"
                + t("USB access uses libaums and libusb, the same USB stack used by EtchDroid."))
        .setPositiveButton(
            t("Licenses"),
            (d, w) -> {
              try {
                String[] names = getAssets().list("licenses");
                new AlertDialog.Builder(this)
                    .setTitle(t("Licenses"))
                    .setItems(
                        names,
                        (dialog, n) -> {
                          try (InputStream in = getAssets().open("licenses/" + names[n])) {
                            ByteArrayOutputStream out = new ByteArrayOutputStream();
                            byte[] b = new byte[4096];
                            int count;
                            while ((count = in.read(b)) != -1) out.write(b, 0, count);
                            new AlertDialog.Builder(this)
                                .setTitle(names[n])
                                .setMessage(out.toString("UTF-8"))
                                .setPositiveButton(t("OK"), null)
                                .show();
                          } catch (Exception e) {
                            showError(e.toString());
                          }
                        })
                    .show();
              } catch (Exception e) {
                showError(e.toString());
              }
            })
        .setNegativeButton(t("Close"), null)
        .show();
  }

  private void info(String title, String message) {
    new AlertDialog.Builder(this)
        .setTitle(t(title))
        .setMessage(t(message))
        .setPositiveButton(t("OK"), null)
        .show();
  }

  private void showError(String message) {
    new AlertDialog.Builder(this)
        .setTitle(t("Operation could not finish"))
        .setMessage(
            t("Check the selected image and card, then try again.")
                + "\n\n"
                + t("Technical details")
                + ":\n"
                + message)
        .setPositiveButton(t("OK"), null)
        .show();
  }
}
