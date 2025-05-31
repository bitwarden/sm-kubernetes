FROM golang:1.22 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY Makefile Makefile

RUN apt update && apt install unzip musl-tools -y

RUN mkdir state

RUN CC=musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags '-linkmode external -extldflags "-static -Wl,-unresolved-symbols=ignore-all"' -o manager cmd/main.go

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder --chown=65532:65532 /workspace/state/ ./state/

USER 65532:65532

ENV BW_SECRETS_MANAGER_STATE_PATH='/state'
ENV CGO_ENABLED=1

ENTRYPOINT ["/manager"]
