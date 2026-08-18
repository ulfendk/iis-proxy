# --- build stage ---
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/iis-proxy .

# --- final stage ---
# distroless "static" has no libc/shell at all, which is fine since the
# binary above is a static, dependency-free build. It also already runs as
# a non-root "nonroot" user (uid 65532). If you ever need a shell inside
# the container to debug, swap this for "alpine:3" + `RUN adduser` instead.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/iis-proxy /iis-proxy
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/iis-proxy"]
