FROM node:22-alpine AS media-assets
WORKDIR /bundle
RUN npm init -y >/dev/null && npm install --no-save --ignore-scripts --no-audit --no-fund livekit-client@2.22.3 \
    && test -s node_modules/livekit-client/dist/livekit-client.umd.js

FROM golang:1.27.1-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /media-provider .

FROM alpine:3.22
WORKDIR /srv
COPY --from=builder /media-provider /usr/local/bin/media-provider
COPY static/ ./static/
COPY --from=media-assets /bundle/node_modules/livekit-client/dist/livekit-client.umd.js ./assets/livekit-client.umd.js
RUN mkdir -p /srv/state && chown -R 65532:65532 /srv/state
USER 65532:65532
ENV APGIC_COMM_REF_LISTEN=0.0.0.0:8650
ENV APGIC_COMM_REF_STATE_DIR=/srv/state
ENV APGIC_COMM_REF_LIVEKIT_CLIENT_BUNDLE=/srv/assets/livekit-client.umd.js
EXPOSE 8650
CMD ["/usr/local/bin/media-provider"]
