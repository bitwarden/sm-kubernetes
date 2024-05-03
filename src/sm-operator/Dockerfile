FROM golang:1.20 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
COPY bw-sdk/ bw-sdk/

RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY Makefile Makefile

RUN apt update && apt install unzip musl-tools npm -y
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y

RUN PATH="$PATH:/root/.cargo/bin" GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} make binaries

RUN mkdir state

RUN CC=musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags '-linkmode external -extldflags "-static"' -o manager cmd/main.go

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder --chown=65532:65532 /workspace/state/ ./state/

USER 65532:65532

ENV BW_SECRETS_MANAGER_STATE_PATH='/state'
ENV CGO_ENABLED=1

ENTRYPOINT ["/manager"]
