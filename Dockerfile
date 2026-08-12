FROM ghcr.io/cirruslabs/flutter:3.44.0 AS flutter-builder

WORKDIR /src/app
COPY app/pubspec.yaml app/pubspec.lock ./
RUN flutter pub get
COPY app/ ./
RUN flutter build web --release

FROM golang:1.25-alpine AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api/ ./api/
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S uav \
    && adduser -S -G uav uav

WORKDIR /app
COPY --from=go-builder /out/server ./server
COPY api/swagger/ ./api/swagger/
COPY --from=flutter-builder /src/app/build/web/ ./app/build/web/

USER uav
EXPOSE 8080 50051
CMD ["./server"]
