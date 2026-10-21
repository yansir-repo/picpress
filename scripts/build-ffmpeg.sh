#!/bin/sh
# Builds the ffmpeg / ffprobe that PicPress bundles for video frame extraction
# (internal/push/video.go): static, LGPL-2.1-or-later only, decode-only, PNG
# out on a pipe. Nothing outside the macOS SDK is linked (system libz only).
#
# Usage: FFMPEG_VERSION=… FFMPEG_SHA256=… MACOSX_DEPLOYMENT_TARGET=… \
#          scripts/build-ffmpeg.sh <arm64|x86_64> <dir>
# Output: <dir>/<arch>/{ffmpeg,ffprobe}, <dir>/COPYING.LGPLv2.1
# x86_64 needs nasm on PATH (brew install nasm).
set -eu

arch=$1
dir=$2
: "${FFMPEG_VERSION:?}" "${FFMPEG_SHA256:?}" "${MACOSX_DEPLOYMENT_TARGET:?}"

case "$arch" in
arm64 | x86_64) ;;
*) echo "unsupported arch: $arch" >&2 && exit 2 ;;
esac

mkdir -p "$dir"
dir=$(cd "$dir" && pwd)
tarball="$dir/ffmpeg-$FFMPEG_VERSION.tar.xz"
if [ ! -f "$tarball" ]; then
	curl -fL --retry 3 -o "$tarball.part" "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz"
	mv "$tarball.part" "$tarball"
fi
echo "$FFMPEG_SHA256  $tarball" | shasum -a 256 -c -

work="$dir/build-$arch"
src="$work/ffmpeg-$FFMPEG_VERSION"
rm -rf "$work"
mkdir -p "$work"
tar -xJf "$tarball" -C "$work"

# Paths stay out of the configure line (embedded in `ffmpeg -version`): the
# SDK comes from SDKROOT, and the in-tree build keeps __FILE__ relative.
SDKROOT=$(xcrun --sdk macosx --show-sdk-path)
export SDKROOT
flags="-arch $arch -mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET"

# Containers and codecs cover mp4 / mov / m4v / avi / mkv / webm / mts.
# extract_extradata is used at run time by avformat_find_stream_info (codec
# parameters for streams without global headers, e.g. H.264 in MPEG-TS).
# Not enabling gpl / version3 / nonfree keeps the result LGPL-2.1-or-later.
cd "$src"
./configure \
	--arch="$arch" --target-os=darwin --enable-cross-compile \
	--cc=clang \
	--extra-cflags="$flags" \
	--extra-ldflags="$flags -Wl,-dead_strip" \
	--enable-static --disable-shared \
	--disable-autodetect --disable-everything \
	--disable-doc --disable-debug --disable-ffplay \
	--disable-avdevice --disable-swresample --disable-network \
	--enable-pthreads --enable-zlib \
	--enable-protocol=file,pipe \
	--enable-demuxer=mov,avi,matroska,mpegts \
	--enable-parser=h264,hevc,mpeg4video,mpegvideo,mjpeg,vp8,vp9,vc1,h263 \
	--enable-decoder=h264,hevc,mpeg4,mpeg1video,mpeg2video,mjpeg,vp8,vp9,prores,h263,msmpeg4v1,msmpeg4v2,msmpeg4v3,vc1,wmv3,dvvideo,rawvideo \
	--enable-bsf=extract_extradata \
	--enable-filter=scale \
	--enable-encoder=png \
	--enable-muxer=image2pipe

if grep -q -E '^#define CONFIG_(GPL|VERSION3|NONFREE) 1' config.h; then
	echo "configured build is not LGPL-2.1-or-later" >&2
	exit 1
fi

make -j"$(sysctl -n hw.ncpu)" ffmpeg ffprobe
mkdir -p "$dir/$arch"
cp ffmpeg ffprobe "$dir/$arch/"
cp COPYING.LGPLv2.1 "$dir/COPYING.LGPLv2.1"
rm -rf "$work"
