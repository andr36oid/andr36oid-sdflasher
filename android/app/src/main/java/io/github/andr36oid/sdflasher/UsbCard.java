package io.github.andr36oid.sdflasher;

import android.content.Context;
import android.hardware.usb.*;
import io.github.andr36oid.mobile.Disk;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.ArrayList;
import java.util.List;
import me.jahnen.libaums.core.driver.BlockDeviceDriver;
import me.jahnen.libaums.core.driver.BlockDeviceDriverFactory;
import me.jahnen.libaums.core.driver.scsi.commands.sense.MediaNotInserted;
import me.jahnen.libaums.core.usb.UsbCommunication;
import me.jahnen.libaums.libusbcommunication.LibusbCommunication;

final class UsbCard {
  final UsbDevice device;
  final UsbInterface iface;
  final UsbEndpoint input, output;
  final int lun;
  final long bytes;

  UsbCard(UsbDevice d, UsbInterface i, UsbEndpoint in, UsbEndpoint out, int l, long size) {
    device = d;
    iface = i;
    input = in;
    output = out;
    lun = l;
    bytes = size;
  }

  String label() {
    String name = device.getProductName();
    if (name == null || name.trim().isEmpty())
      name = "USB " + device.getVendorId() + ":" + device.getProductId();
    return name
        + " · "
        + device.getDeviceName()
        + (lun >= 0 ? " · LUN " + lun : "")
        + (bytes > 0 ? String.format(java.util.Locale.ROOT, " · %.1f GB", bytes / 1e9) : "");
  }

  static List<UsbCard> readers(Context context) {
    UsbManager manager = (UsbManager) context.getSystemService(Context.USB_SERVICE);
    List<UsbCard> list = new ArrayList<>();
    for (UsbDevice device : manager.getDeviceList().values()) {
      for (int n = 0; n < device.getInterfaceCount(); n++) {
        UsbInterface iface = device.getInterface(n);
        if (iface.getInterfaceClass() != 8
            || iface.getInterfaceSubclass() != 6
            || iface.getInterfaceProtocol() != 80) continue;
        UsbEndpoint in = null, out = null;
        for (int k = 0; k < iface.getEndpointCount(); k++) {
          UsbEndpoint ep = iface.getEndpoint(k);
          if (ep.getType() != UsbConstants.USB_ENDPOINT_XFER_BULK) continue;
          if (ep.getDirection() == UsbConstants.USB_DIR_IN) in = ep;
          else out = ep;
        }
        if (in != null && out != null) list.add(new UsbCard(device, iface, in, out, -1, 0));
      }
    }
    return list;
  }

  UsbCommunication connect(Context context) throws IOException {
    UsbManager manager = (UsbManager) context.getSystemService(Context.USB_SERVICE);
    UsbDevice current = manager.getDeviceList().get(device.getDeviceName());
    if (current == null
        || current.getDeviceId() != device.getDeviceId()
        || !manager.hasPermission(current))
      throw new IOException("USB device disconnected or permission was revoked");
    return new LibusbCommunication(manager, current, iface, output, input);
  }

  List<UsbCard> discover(Context context) throws IOException {
    try (UsbCommunication usb = connect(context)) {
      byte[] max = new byte[1];
      // A stalled GET_MAX_LUN means a single logical unit in the BOT specification.
      try {
        if (usb.controlTransfer(0xa1, 0xfe, 0, iface.getId(), max, 1) != 1) max[0] = 0;
      } catch (IOException e) {
        max[0] = 0;
      }
      int count = (max[0] & 255);
      if (count > 15) throw new IOException("Invalid USB logical unit count");
      List<UsbCard> cards = new ArrayList<>();
      for (int l = 0; l <= count; l++) {
        BlockDeviceDriver driver =
            BlockDeviceDriverFactory.INSTANCE.createBlockDevice(usb, (byte) l);
        try {
          driver.init();
        } catch (MediaNotInserted empty) {
          continue;
        }
        if (driver.getBlockSize() != 512 || driver.getBlocks() == -1L) continue;
        long size = capacityBytes(driver);
        if (size >= 1L << 30) cards.add(new UsbCard(device, iface, input, output, l, size));
      }
      return cards;
    }
  }

  static long capacityBytes(BlockDeviceDriver driver) throws IOException {
    // This pinned libaums version exposes READ CAPACITY(10)'s last LBA,
    // including its signed Java representation, rather than a block count.
    long last = driver.getBlocks() & 0xffffffffL;
    if (driver.getBlockSize() != 512 || last == 0xffffffffL)
      throw new IOException("Unsupported logical sector size or READ CAPACITY(16) device");
    return (last + 1) * 512L;
  }

  Connection open(Context context) throws Exception {
    UsbCommunication usb = connect(context);
    try {
      BlockDeviceDriver driver =
          BlockDeviceDriverFactory.INSTANCE.createBlockDevice(usb, (byte) lun);
      driver.init();
      if (driver.getBlockSize() != 512 || capacityBytes(driver) != bytes)
        throw new IOException("Card capacity changed; select it again");
      String serial = device.getSerialNumber();
      String id =
          device.getVendorId()
              + ":"
              + device.getProductId()
              + ":"
              + (serial == null ? "" : serial)
              + ":"
              + iface.getId()
              + ":"
              + lun;
      Connection connection = new Connection(usb, driver, bytes, id, lun);
      connection.flush();
      return connection;
    } catch (Exception e) {
      usb.close();
      throw e;
    }
  }

  static final class Connection implements Disk, AutoCloseable {
    private final UsbCommunication usb;
    private final BlockDeviceDriver driver;
    private final long bytes;
    private final String id;
    private final int lun;
    private boolean closed = false, flushSupported = true;
    private int tag = 0x53444600;

    Connection(UsbCommunication u, BlockDeviceDriver d, long size, String identity, int unit) {
      usb = u;
      driver = d;
      bytes = size;
      id = identity;
      lun = unit;
    }

    @Override
    public long capacity() {
      return bytes;
    }

    @Override
    public String identity() {
      return id;
    }

    private void range(long off, long length) throws IOException {
      if (closed
          || off < 0
          || length < 0
          || off > bytes
          || length > bytes - off
          || off % 512 != 0
          || length % 512 != 0
          || length > 4L << 20) throw new IOException("Invalid USB transfer range");
    }

    @Override
    public synchronized byte[] read(long off, long length) throws Exception {
      range(off, length);
      byte[] result = new byte[(int) length];
      driver.read(off / 512, ByteBuffer.wrap(result));
      return result;
    }

    @Override
    public synchronized void write(long off, byte[] data) throws Exception {
      range(off, data.length);
      driver.write(off / 512, ByteBuffer.wrap(data));
    }

    private int command(byte[] cdb, byte[] data) throws IOException {
      ByteBuffer cbw = ByteBuffer.allocate(31).order(ByteOrder.LITTLE_ENDIAN);
      int current = ++tag;
      cbw.putInt(0x43425355)
          .putInt(current)
          .putInt(data.length)
          .put((byte) (data.length > 0 ? 0x80 : 0))
          .put((byte) lun)
          .put((byte) cdb.length)
          .put(cdb);
      cbw.rewind();
      if (usb.bulkOutTransfer(cbw) != 31) throw new IOException("Short SCSI command transfer");
      if (data.length > 0) {
        ByteBuffer buffer = ByteBuffer.wrap(data);
        while (buffer.hasRemaining())
          if (usb.bulkInTransfer(buffer) <= 0) throw new IOException("Short SCSI data transfer");
      }
      ByteBuffer csw = ByteBuffer.allocate(13).order(ByteOrder.LITTLE_ENDIAN);
      if (usb.bulkInTransfer(csw) != 13) throw new IOException("Short SCSI status transfer");
      csw.rewind();
      if (csw.getInt() != 0x53425355 || csw.getInt() != current || csw.getInt() != 0)
        throw new IOException("Invalid SCSI command status");
      return (csw.get() & 255);
    }

    @Override
    public synchronized void flush() throws Exception {
      if (closed) throw new IOException("USB connection closed");
      if (!flushSupported) return;
      byte[] cdb = new byte[10];
      cdb[0] = 0x35;
      int status = command(cdb, new byte[0]);
      if (status == 0) return;
      if (status == 1) {
        byte[] sense = new byte[18];
        if (command(new byte[] {3, 0, 0, 0, 18, 0}, sense) == 0
            && (sense[2] & 15) == 5
            && (sense[12] == 0x20 || sense[12] == 0x24)) {
          // Some flash readers do not implement cache commands. All writes are
          // synchronous USB commands and read-back still bypasses host caches.
          flushSupported = false;
          return;
        }
      }
      throw new IOException("The card could not flush its write cache");
    }

    @Override
    public synchronized void close() throws IOException {
      if (!closed) {
        closed = true;
        usb.close();
      }
    }
  }
}
