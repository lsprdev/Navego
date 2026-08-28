# syntax=docker/dockerfile:1

FROM lscr.io/linuxserver/chromium:version-be140933@sha256:4c7b9086d2e5054b61c9e6b0f75efceece5befeff2bec3ff543d52e000cee21d

USER root

WORKDIR /opt/navego-browser

COPY docker/start-browser ./start-browser
COPY docker/autostart /defaults/autostart
COPY docker/autostart-wayland /defaults/autostart_wayland
COPY docker/install-autostart /custom-cont-init.d/50-navego-autostart
RUN chmod 0755 \
    ./start-browser \
    /defaults/autostart \
    /defaults/autostart_wayland \
    /custom-cont-init.d/50-navego-autostart

EXPOSE 3000 3001
