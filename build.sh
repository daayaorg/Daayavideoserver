#get the version from the file 'VERSION'
VERSION=$(head -1 VERSION)

# Run tests
echo "Running tests..."
if ! go test ./...; then
    echo "Tests failed. Aborting build."
    exit 1
fi

echo "Building binary..."
# Build with hardening flags
CGO_ENABLED=0 go build -ldflags="-s -w -extldflags=-Wl,-z,relro,-z,now" -buildmode=pie

# Strip binary to reduce size and remove debug symbols
echo "Stripping binary..."
strip daayavideoserver 2>/dev/null || echo "Warning: strip command not available, binary not stripped"


### create debian package
rm -rf   daayavideoservice
rm -rf   daayavideoservice.deb
mkdir -p daayavideoservice/opt/daayavideoservice
mkdir -p daayavideoservice/DEBIAN

cp DEBIAN/postinst daayavideoservice/DEBIAN
cp DEBIAN/prerm daayavideoservice/DEBIAN
cp DEBIAN/daayavideo.service daayavideoservice/opt/daayavideoservice
cp daayavideoserver daayavideoservice/opt/daayavideoservice

# Set proper permissions
chmod 755 daayavideoservice/DEBIAN/postinst daayavideoservice/DEBIAN/prerm
chmod 755 daayavideoservice/opt/daayavideoservice/daayavideoserver
chmod 644 daayavideoservice/opt/daayavideoservice/daayavideo.service
chmod 755 daayavideoservice daayavideoservice/opt daayavideoservice/opt/daayavideoservice daayavideoservice/DEBIAN

cat << EOF > daayavideoservice/DEBIAN/control
Package: daayavideoservice
Version: $(head -1 VERSION)
Architecture: amd64
Section: video
Priority: optional
Depends: libc6 (>= 2.31)
Maintainer: JP Brahma <jp@daaya.org>
Homepage:https://www.daaya.org/
Description: Daaya Video Server
 Educational video streaming server with taxonomic classification.
 Provides REST API for streaming educational videos organized by
 hierarchical taxonomy (e.g., elementary/math/number system/counting).
EOF
# Build package with fakeroot for proper ownership
fakeroot dpkg-deb --build daayavideoservice 2>/dev/null || dpkg-deb --build daayavideoservice

#version the debian package
mv daayavideoservice.deb daayavideoservice-"$VERSION".deb

rm -rf daayavideoservice
