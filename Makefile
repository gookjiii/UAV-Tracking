.PHONY: proto build run test test-go test-flutter analyze web macos docker-up docker-down clean

PROTO_IMAGE=uav-proto-tools:local
FLUTTER_DIR=app

proto:
	docker build -f build/proto.Dockerfile -t $(PROTO_IMAGE) .
	docker run --rm -v "$(PWD):/workspace" -w /workspace $(PROTO_IMAGE) \
		-I api/proto -I /usr/local/include \
		--go_out=. --go_opt=module=github.com/uav_tracking \
		--go-grpc_out=. --go-grpc_opt=module=github.com/uav_tracking \
		--grpc-gateway_out=. --grpc-gateway_opt=module=github.com/uav_tracking \
		--openapiv2_out=api/swagger --openapiv2_opt=logtostderr=true \
		api/proto/drone/drone.proto

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/server ./cmd/server

run: build
	./bin/server

test: test-go test-flutter

test-go:
	go test -race ./...
	go vet ./...

test-flutter:
	cd $(FLUTTER_DIR) && flutter test

analyze:
	cd $(FLUTTER_DIR) && flutter analyze

web:
	cd $(FLUTTER_DIR) && flutter build web --release

macos:
	cd $(FLUTTER_DIR) && flutter build macos --release

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

clean:
	go clean
	cd $(FLUTTER_DIR) && flutter clean
	rm -f bin/server
