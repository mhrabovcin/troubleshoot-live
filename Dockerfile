# syntax=docker/dockerfile:1

# Use distroless/static:nonroot image for a base.
FROM --platform=linux/amd64 gcr.io/distroless/static@sha256:8d4cc4a622ce09a75bd7b1eea695008bdbff9e91fea426c2d353ea127dcdc9e3 as linux-amd64
FROM --platform=linux/arm64 gcr.io/distroless/static@sha256:31b88f1a22bd3676d8d2fad1022e06ce5ee1a66de896fd2cc141746f2681ae2f as linux-arm64

FROM --platform=linux/${TARGETARCH} linux-${TARGETARCH}

# Run as nonroot user using numeric ID for compatibllity.
USER 65532

COPY troubleshoot-live /usr/local/bin/troubleshoot-live

ENTRYPOINT ["/usr/local/bin/troubleshoot-live"]
