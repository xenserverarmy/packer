#!/bin/bash
#
# This script builds the application from source for multiple platforms.
# Adapted from from packer/scripts/build.sh

# Determine the arch/os combos we're building for
XC_OS=${XC_OS:-$(go env GOOS)}
XC_ARCH=${XC_ARCH:-$(go env GOARCH)}
GOPATH=${GOPATH:-$(go env GOPATH)}

# Install dependencies

# Delete the old dir
echo "==> Removing old directory..."
rm -f bin/*
rm -rf pkg/*
rm -rf ${GOPATH}/pkg/*
mkdir -p bin/

# Install gox, if not already present
go get github.com/mitchellh/gox

gox \
    -os="${XC_OS}" \
    -arch="${XC_ARCH}" \
    -output "pkg/{{.OS}}_{{.Arch}}/packer-{{.Dir}}" \
    ./... \
    || exit 1

# Move all the compiled things to the $GOPATH/bin
case $(uname) in
    CYGWIN*)
        GOPATH="$(cygpath $GOPATH)"
        ;;
esac
OLDIFS=$IFS
IFS=: MAIN_GOPATH=($GOPATH)
IFS=$OLDIFS

# Copy our OS/Arch to the bin/ directory
echo "==> Copying binaries for this platform..."
DEV_PLATFORM="./pkg/${XC_OS}_${XC_ARCH}"
for F in $(find ${DEV_PLATFORM} -mindepth 1 -maxdepth 1 -type f); do
    cp -v ${F} bin/
    cp -v ${F} ${MAIN_GOPATH}/bin/
done

# Done!
echo
echo "==> Results:"
ls -ahl bin/
