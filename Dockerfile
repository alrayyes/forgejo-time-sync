FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/forgejo-time-sync ./cmd/forgejo-time-sync

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/forgejo-time-sync /forgejo-time-sync
VOLUME /data
ENTRYPOINT ["/forgejo-time-sync"]
