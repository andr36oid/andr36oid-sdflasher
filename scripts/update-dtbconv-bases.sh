#!/bin/sh
# Refresh the base DTBs the DTB converter (internal/dtbconv) starts from.
#
# usage: scripts/update-dtbconv-bases.sh <kernel dtb dir> <device BOOT/uboot-dtb> [kernel commit]
#   <kernel dtb dir>  arch/arm64/boot/dts/rockchip of a kernel build (make dtbs)
#                     of kernel/gameconsole/r36s, branch night/integration or newer
#   <uboot-dtb>       device/gameconsole/r36s BOOT/uboot-dtb
#
# Bases: r36s = rk3326-r36s-android-panel4.dtb (genuine R36S wiring),
#        ee   = rk3326-r36s-clone-g80-panel8.dtb (EE clone, G80 wiring),
#        soy  = rk3326-r36s-clone-soysauce-panel5.dtb (Soy Sauce),
#        uboot = BOOT/uboot-dtb (R36S U-Boot panel DTB).
set -eu
[ $# -ge 2 ] || { sed -n '2,13p' "$0"; exit 2; }
k=$1 u=$2 rev=${3:-unknown}
out=$(dirname "$0")/../internal/dtbconv/bases
for pair in r36s:rk3326-r36s-android-panel4 ee:rk3326-r36s-clone-g80-panel8 soy:rk3326-r36s-clone-soysauce-panel5; do
	name=${pair%%:*} file=${pair#*:}
	gzip -9n < "$k/$file.dtb" > "$out/$name.dtb.gz"
done
gzip -9n < "$u" > "$out/uboot.dtb.gz"
{
	echo "Base DTBs of internal/dtbconv, made by scripts/update-dtbconv-bases.sh."
	echo "kernel: $rev"
	echo "r36s.dtb.gz  = rk3326-r36s-android-panel4.dtb"
	echo "ee.dtb.gz    = rk3326-r36s-clone-g80-panel8.dtb"
	echo "soy.dtb.gz   = rk3326-r36s-clone-soysauce-panel5.dtb"
	echo "uboot.dtb.gz = device/gameconsole/r36s BOOT/uboot-dtb"
	for f in "$out"/*.dtb.gz; do
		printf '%s sha256 %s\n' "$(basename "$f" .gz)" "$(gzip -dc "$f" | sha256sum | cut -d' ' -f1)"
	done
} > "$out/SOURCES.txt"
