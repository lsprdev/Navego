# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/navego ./cmd/navego

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/navego /navego

EXPOSE 8001
USER nonroot:nonroot
ENTRYPOINT ["/navego"]
