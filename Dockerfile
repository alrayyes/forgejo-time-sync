FROM golang:1.27.1@sha256:512690a5660563b57d37ecc31129e7f136e831db2aed24a1dbeb8ad7380dc0fa AS build
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

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/forgejo-time-sync /forgejo-time-sync
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME /data
# Exec form, calling the binary itself with its "healthcheck" argument —
# distroless has no shell or curl for a CMD-SHELL/curl-style check to run.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD ["/forgejo-time-sync", "healthcheck"]
ENTRYPOINT ["/forgejo-time-sync"]
