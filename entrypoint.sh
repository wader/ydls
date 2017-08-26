#!/bin/sh

if [ "$1" = "" ] ; then
  exec tini -- ydls-server -info -formats "$FORMATS" -listen "$LISTEN"
else
  exec ydls-get -formats "$FORMATS" "$@"
fi
