#!/bin/sh
eth1=`ip -o -4 addr list eth1 | awk '{print $4}' | cut -d/ -f1`
cat $1 | sed  "s/XXX_HOST_IP/$eth1/" > $2
