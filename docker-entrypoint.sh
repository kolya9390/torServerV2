#!/bin/sh

# TorrServer v3.0 — Entry Point Script
# Minimal entry point for Docker

FLAGS="--path $TS_CONF_PATH --logpath $TS_LOG_PATH --port $TS_PORT --torrentsdir $TS_TORR_DIR"

# Network settings
if [ -n "$TS_IP" ]; then FLAGS="${FLAGS} -i ${TS_IP}"; fi
if [ -n "$TS_PUBIPV4" ]; then FLAGS="${FLAGS} --pubipv4 ${TS_PUBIPV4}"; fi
if [ -n "$TS_PUBIPV6" ]; then FLAGS="${FLAGS} --pubipv6 ${TS_PUBIPV6}"; fi

# SSL settings
if [ "$TS_EN_SSL" = "1" ]; then FLAGS="${FLAGS} --ssl"; fi
if [ -n "$TS_SSL_PORT" ]; then FLAGS="${FLAGS} --sslport ${TS_SSL_PORT}"; fi
if [ -n "$TS_SSL_CERT" ]; then FLAGS="${FLAGS} --sslcert ${TS_SSL_CERT}"; fi
if [ -n "$TS_SSL_KEY" ]; then FLAGS="${FLAGS} --sslkey ${TS_SSL_KEY}"; fi

# Proxy settings
if [ -n "$TS_PROXYURL" ]; then FLAGS="${FLAGS} --proxyurl ${TS_PROXYURL}"; fi
if [ -n "$TS_PROXYMODE" ]; then FLAGS="${FLAGS} --proxymode ${TS_PROXYMODE}"; fi

# Torrent settings
if [ -n "$TS_MAXSIZE" ]; then FLAGS="${FLAGS} --maxsize ${TS_MAXSIZE}"; fi
if [ -n "$TS_TORRENTADDR" ]; then FLAGS="${FLAGS} --torrentaddr ${TS_TORRENTADDR}"; fi

# FUSE/WebDAV
if [ "$TS_WEBDAV" = "1" ]; then FLAGS="${FLAGS} --webdav"; fi
if [ -n "$TS_FUSEPATH" ]; then FLAGS="${FLAGS} --fusepath ${TS_FUSEPATH}"; fi

# Create directories
if [ ! -d "$TS_CONF_PATH" ]; then
  mkdir -p "$TS_CONF_PATH"
fi

if [ ! -d "$TS_TORR_DIR" ]; then
  mkdir -p "$TS_TORR_DIR"
fi

if [ ! -d "$TS_LOG_PATH" ]; then
  mkdir -p "$TS_LOG_PATH"
fi

echo "Running TorrServer v3.0 with: $FLAGS"

exec torrserver $FLAGS
