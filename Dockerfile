# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pb_migrations ./pb_migrations
RUN --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/navego-worker ./cmd/navego
RUN --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/navego-control ./cmd/navego-control
RUN --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/navego-agent ./cmd/navego-agent

FROM gcr.io/distroless/static-debian12:latest AS control

COPY --from=build /out/navego-control /navego-control

EXPOSE 8090
USER root:root
ENTRYPOINT ["/navego-control"]

FROM gcr.io/distroless/static-debian12:latest AS agent

COPY --from=build /out/navego-agent /navego-agent
COPY --from=build /out/navego-worker /navego-worker

USER root:root
ENTRYPOINT ["/navego-agent"]

FROM gcr.io/distroless/static-debian12:nonroot AS worker

COPY --from=build /out/navego-worker /navego-worker

EXPOSE 8001
USER nonroot:nonroot
ENTRYPOINT ["/navego-worker"]
