BINARY=binance_bot
BUILD_DIR=./build

.PHONY: build run clean tidy

build:
	@echo "编译中..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/main.go
	@echo "编译完成: $(BUILD_DIR)/$(BINARY)"

run:
	go run ./cmd/main.go

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy

test:
	go test ./...
