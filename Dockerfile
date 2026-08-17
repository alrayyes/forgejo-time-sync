FROM golang:1.26.6@sha256:0d1d3a794be25f809dd2cb3160d8c73276c4056a9f8242a138e908ddeee7b6b6 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/forgejo-time-sync ./cmd/forgejo-time-sync
# A fresh named/anonymous volume is seeded from whatever already exists in
# the image at its mount point — owner included. Without this, Docker
# creates /data itself on first mount, owned by root:root, which the
# nonroot (uid 65532) user below can't write to: state.json would fail to
# save on every single write.
RUN mkdir /out/data && chown 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a
COPY --from=build /out/forgejo-time-sync /forgejo-time-sync
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME /data
# Exec form, calling the binary itself with its "healthcheck" argument —
# distroless has no shell or curl for a CMD-SHELL/curl-style check to run.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD ["/forgejo-time-sync", "healthcheck"]
ENTRYPOINT ["/forgejo-time-sync"]
