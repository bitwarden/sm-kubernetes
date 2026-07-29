# base-builder holds everything shared by every binary built from this
# repository: the Go module cache and the musl-gcc toolchain used to produce
# a static binary that still CGO-links against the Bitwarden SDK's C library
# (github.com/bitwarden/sdk-go/v2).
FROM golang:1.26 AS base-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

RUN apt update && apt install unzip musl-tools -y

# ---- sm-operator controller manager ----
FROM base-builder AS manager-builder

COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY internal/sm/ internal/sm/
COPY Makefile Makefile

RUN mkdir state

RUN CC=musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags '-linkmode external -extldflags "-static -Wl,-unresolved-symbols=ignore-all"' -o manager cmd/main.go

# ---- bitwarden secrets-store-csi-driver provider ----
FROM base-builder AS csi-provider-builder

ARG VERSION=dev

COPY cmd/csi-provider/ cmd/csi-provider/
COPY api/ api/
COPY internal/csi/ internal/csi/
COPY internal/sm/ internal/sm/

RUN CC=musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags "-linkmode external -extldflags \"-static -Wl,-unresolved-symbols=ignore-all\" -X github.com/bitwarden/sm-kubernetes/internal/csi.Version=${VERSION}" -o csi-provider ./cmd/csi-provider

# csi-provider is a non-default build target; build it explicitly with:
#   docker build --target csi-provider -t <tag> .
FROM gcr.io/distroless/base-debian12:nonroot AS csi-provider
WORKDIR /
COPY --from=csi-provider-builder /workspace/csi-provider .

USER 65532:65532

ENV CGO_ENABLED=1

ENTRYPOINT ["/csi-provider"]

# manager is the default build target (the last stage, built when no
# --target is given), preserving prior `docker build .` behavior.
FROM gcr.io/distroless/base-debian12:nonroot AS manager
WORKDIR /
COPY --from=manager-builder /workspace/manager .
COPY --from=manager-builder --chown=65532:65532 /workspace/state/ ./state/

USER 65532:65532

ENV BW_SECRETS_MANAGER_STATE_PATH='/state'
ENV CGO_ENABLED=1

ENTRYPOINT ["/manager"]
