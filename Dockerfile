# syntax=docker/dockerfile:1

# The builder cross-compiles for the target platform, so multi-arch images do
# not need emulation. The binary is pure Go and has no runtime dependencies.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/helm-schema ./cmd/helm-schema

# Alpine keeps /bin/sh so the image also works as a CI job image.
FROM alpine:3.24
RUN apk add --no-cache ca-certificates
COPY --from=build /out/helm-schema /usr/local/bin/helm-schema
WORKDIR /chart
ENTRYPOINT ["helm-schema"]
CMD ["."]
