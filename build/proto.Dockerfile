FROM golang:1.25-alpine

ARG TARGETARCH
ARG PROTOC_VERSION=25.3

RUN apk add --no-cache curl unzip \
    && case "$TARGETARCH" in \
         amd64) PROTOC_ARCH=x86_64 ;; \
         arm64) PROTOC_ARCH=aarch_64 ;; \
         *) echo "Unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
       esac \
    && curl -fsSL "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${PROTOC_ARCH}.zip" -o /tmp/protoc.zip \
    && unzip -q /tmp/protoc.zip -d /usr/local \
    && rm /tmp/protoc.zip \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.33.0 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.30.0 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.30.0

ENTRYPOINT ["protoc"]
