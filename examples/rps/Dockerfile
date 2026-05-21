# syntax=docker/dockerfile:1.7
#
# Builds an Agones-ready room image: the stock vfx binary plus the RPS
# plugin compiled to WASM, baked in so a vanilla `vfx room` can load it
# via VFX_ROOM_PLUGIN_PATH. The same image serves the gateway (it just
# carries an extra file the gateway never reads).
#
# Build (the build context must contain rps.wasm; bin/ works after
# `mise run build-rps-wasm`):
#
#	docker build -f examples/rps/Dockerfile \
#	  --build-arg VFX_IMAGE=vfx:dev -t vfx-rps:dev bin
ARG VFX_IMAGE=ghcr.io/averak/vfx:latest
FROM ${VFX_IMAGE}

COPY rps.wasm /opt/vfx/plugins/rps.wasm
