# 编译命令 (需要 CGO 支持 SQLite)

.PHONY: build build-linux build-all clean

# 默认编译当前平台
build:
	CGO_ENABLED=1 go build -o flowgate -ldflags '-s -w' ./cmd/flowgate/

# 编译 Linux amd64 (最常用)
build-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o flowgate-linux-amd64 -ldflags '-s -w' ./cmd/flowgate/

# 编译 Linux arm64
build-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o flowgate-linux-arm64 -ldflags '-s -w' ./cmd/flowgate/

# 编译所有平台
build-all: build-linux build-linux-arm64

# 运行面板 (开发模式)
dev-panel:
	go run ./cmd/flowgate/ panel --port 8080

# 运行节点 (开发模式)
dev-node:
	go run ./cmd/flowgate/ node --panel ws://localhost:8080/ws/node --key $(KEY)

# 清理
clean:
	rm -f flowgate flowgate-linux-* flowgate.db

# 安装依赖
deps:
	go mod tidy
	go mod download
