# One binary with the site inside it, on one machine, with one volume.
#
# The frontend is built first and embedded into the Go binary, so the
# deployed artifact has no static file server and no second origin.

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --silent
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
# refuse to ship a binary that would serve an empty page
RUN test -f web/dist/index.html || (echo "the frontend build is missing" && exit 1)
# CGO off: modernc.org/sqlite is pure Go, which is why it was chosen
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kindness-chain ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates su-exec && adduser -D -H -u 10001 chain
COPY --from=build /out/kindness-chain /usr/local/bin/kindness-chain
# The volume is mounted by the platform and owned by root, so the
# process starts as root only to hand the mount to its own user.
RUN printf '#!/bin/sh\nset -e\nmkdir -p /data\nchown -R chain:chain /data\nexec su-exec chain kindness-chain\n' > /usr/local/bin/start \
    && chmod +x /usr/local/bin/start
ENV PORT=8080 DB_PATH=/data/chain.db
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/start"]
