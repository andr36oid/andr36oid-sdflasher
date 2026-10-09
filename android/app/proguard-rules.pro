# Go invokes these interface methods by name through JNI, including on lambdas.
-keepclassmembers class * implements io.github.andr36oid.bindings.mobile.Disk {
    public long capacity();
    public java.lang.String identity();
    public byte[] read(long, long);
    public void write(long, byte[]);
    public void flush();
}
-keepclassmembers class * implements io.github.andr36oid.bindings.mobile.Observer {
    public void progress(java.lang.String);
}
