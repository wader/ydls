#!/bin/bash

if [ "$1" = "" ] ; then
  exec tini -- ydls-server -info -formats /etc/formats.json -listen :8080
else
  exec ydls-get -formats /etc/formats.json "$@"
fi
