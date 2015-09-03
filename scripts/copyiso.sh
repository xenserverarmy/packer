#!/bin/sh 
# download an ISO to our library

HOSTNAME=$(hostname)
HOSTUUID=$(xe host-list name-label=$HOSTNAME --minimal)

SRUUID=$(xe sr-list type=iso name-label="$1" --minimal)
ISOPATH=$(xe pbd-list host-uuid=$HOSTUUID sr-uuid=$SRUUID params=device-config --minimal | egrep -o 'iso_path: /(.+);*$' | cut -d':' -f2 | tr -d '[[:space:]]')
#echo "found $SRUUID path '$ISOPATH' for '$1'"

DLPATH=$(echo "/var/run/sr-mount/$SRUUID$ISOPATH/$2")

#Download only if this exists
if ! [ -f $DLPATH ]; then
	curl -f --show-error -s -o $DLPATH $3
fi

