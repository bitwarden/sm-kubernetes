# Build the manager binary
FROM golang:1.20 as builder
ARG TARGETOS
ARG TARGETARCH


# RUN apt update && apt install musl-tools gcc build-essential libc6-dev -y

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY bw-sdk/ bw-sdk/

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
RUN mkdir state

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
# RUN CC=/usr/bin/x86_64-linux-musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a --ldflags '-linkmode external -extldflags "-static"' -o manager cmd/main.go
# RUN CC=/usr/bin/x86_64-linux-musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -tags netgo,osusergo --ldflags '-linkmode external -extldflags "-static"' -o manager cmd/main.go
# RUN CC=/usr/bin/x86_64-linux-musl-gcc CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -tags "cgo netcgo osusergo" -ldflags "-linkmode=external -extldflags '-static-libgcc -static-libstdc++'" -o manager cmd/main.go

RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# RUN CC=/usr/bin/x86_64-linux-musl-gcc CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -tags "cgo netgo osusergo static_build" -ldflags "-s" -o manager cmd/main.go
# RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a --ldflags '-w -extldflags "-static"' -o manager cmd/main.go
# RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -tags static,system_libgit2 -a -o manager cmd/main.go

ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:"/bw-sdk/internal/cinterface/lib/"
RUN ldd manager

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# FROM gcr.io/distroless/base:nonroot
# WORKDIR /
# COPY --from=builder /workspace/manager .
# COPY --from=builder /workspace/bw-sdk .
# COPY --from=builder --chown=65532:65532 /workspace/state/ .
# USER 65532:65532
# ENV BW_SECRETS_MANAGER_STATE_PATH='/state'
# # ENV LD_LIBRARY_PATH $LD_LIBRARY_PATH:'/bw-sdk/internal/cinterface/lib/'
# # ENV CGO_ENABLED=1
# # RUN ['/sbin/ldconfig']
# ENTRYPOINT ["/manager"]



# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM debian:latest
ARG USER=default
ENV HOME /home/$USER

# RUN adduser -D $USER

# USER $USER
# WORKDIR $HOME

COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/bw-sdk .
# COPY --from=builder --chown=$USER:$USER /workspace/state/ .
# USER $USER

ENV BW_SECRETS_MANAGER_STATE_PATH='/state'
# ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:"$HOME/bw-sdk/internal/cinterface/lib/"
ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:"/bw-sdk/internal/cinterface/lib/"
ENV CGO_ENABLED=1
# RUN ldconfig
RUN ldd /manager

ENTRYPOINT ["/manager"]