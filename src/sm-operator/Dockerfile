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
RUN mkdir state

RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go
ENV LD_LIBRARY_PATH /workspace/bw-sdk/internal/cinterface/lib/:$LD_LIBRARY_PATH
RUN mkdir ./bw-sdk-lib
RUN ldd manager | tr -s '[:blank:]' '\n' | grep '^/' | xargs -I % cp % ./bw-sdk-lib
RUN ls ./bw-sdk-lib


FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder --chown=65532:65532 /workspace/state/ ./state/
COPY --from=builder /workspace/bw-sdk-lib ./bw-sdk-lib/

USER 65532:65532

ENV BW_SECRETS_MANAGER_STATE_PATH='/state'
ENV LD_LIBRARY_PATH /bw-sdk-lib/:$LD_LIBRARY_PATH
ENV CGO_ENABLED=1

ENTRYPOINT ["/manager"]
