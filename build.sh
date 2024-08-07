#get the version from the file 'VERSION'
VERSION=$(head -1 VERSION)

go build


### create debian package
rm -rf   daayavideoservice
rm -rf   daayavideoservice.deb
mkdir -p daayavideoservice/opt/daayavideoservice
mkdir -p daayavideoservice/DEBIAN

cp DEBIAN/postinst daayavideoservice/DEBIAN
cp DEBIAN/daayavideo.service daayavideoservice/opt/daayavideoservice
cp daayavideoserver daayavideoservice/opt/daayavideoservice

cat << EOF > daayavideoservice/DEBIAN/control
Package: daayavideoservice
Version: $(head -1 VERSION)
Architecture: amd64
Section: development
Priority: optional
Depends: libc6 (>= 2.31)
Maintainer: JP Brahma <jp@daaya.org>
Homepage:https://www.daaya.org/
Description: Daaya Video Server
EOF
dpkg-deb --build daayavideoservice

#version the debian package
mv daayavideoservice.deb daayavideoservice-"$VERSION".deb

rm -rf daayavideoservice
