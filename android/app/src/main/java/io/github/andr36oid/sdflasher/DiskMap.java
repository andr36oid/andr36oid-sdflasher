package io.github.andr36oid.sdflasher;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.view.View;
import java.util.Arrays;
import org.json.*;

final class DiskMap extends View {
  private final int[] cells = new int[36 * 14];
  private final int[] colors = {
    0xffeeeeee, 0xffaaaaaa, 0xff00cccc, 0xff163caf, 0xffed2020, 0xff0076ff
  };
  private final Paint paint = new Paint();
  private long size;
  private JSONObject currentPlan;

  DiskMap(Context context) {
    super(context);
    setMinimumHeight((int) (140 * getResources().getDisplayMetrics().density));
  }

  private void mark(long off, long length, int state) {
    if (size <= 0 || length <= 0) return;
    int start = (int) Math.max(0, Math.floor((double) off / size * cells.length));
    int end =
        (int) Math.min(cells.length, Math.ceil((double) (off + length) / size * cells.length));
    for (int n = start; n < end; n++) if (cells[n] != 2) cells[n] = state;
  }

  void update(JSONObject plan, JSONObject progress) {
    if (plan != null && plan != currentPlan) {
      currentPlan = plan;
      size = plan.optLong("size");
      Arrays.fill(cells, 0);
      for (String type : new String[] {"writes", "protected"}) {
        JSONArray ranges = plan.optJSONArray(type);
        if (ranges == null) continue;
        for (int n = 0; n < ranges.length(); n++) {
          JSONObject range = ranges.optJSONObject(n);
          if (range != null)
            mark(range.optLong("offset"), range.optLong("length"), type.equals("writes") ? 1 : 2);
        }
      }
    }
    if (progress != null) {
      String phase = progress.optString("phase");
      long off = progress.optLong("offset"), length = progress.optLong("length");
      if (phase.equals("writing")) {
        mark(off, length, 5);
        long block = Math.max(1, size / cells.length);
        mark(off + length - block, block, 4);
      }
      if (phase.equals("verifying")) mark(off, length, 3);
    }
    invalidate();
  }

  @Override
  protected void onDraw(Canvas c) {
    super.onDraw(c);
    float w = getWidth() / 36f, h = getHeight() / 14f;
    for (int n = 0; n < cells.length; n++) {
      paint.setColor(colors[cells[n]]);
      float x = (n % 36) * w, y = (n / 36) * h;
      c.drawRect(x + 1, y + 1, x + w - 1, y + h - 1, paint);
    }
  }
}
