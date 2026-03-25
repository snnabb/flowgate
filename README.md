# ⚡ FlowGate

**轻量级端口流量转发管理面板**

FlowGate 是一款自研的轻量级端口转发管理面板，单二进制文件部署，无需 Docker 或外部数据库依赖。

## ✨ 核心特性

- **极致轻量**: 单二进制文件，Panel < 30MB 内存，Node < 15MB 内存
- **TCP/UDP 转发**: 支持 TCP、UDP、TCP+UDP 三种协议转发
- **Phase 1 隧道引擎**: 支持 TLS、WebSocket、连接池、协议阻断、PROXY Protocol
- **实时管理**: 规则增删改实时生效，无需重启
- **多节点管理**: Panel + Node Agent 架构，统一管理多台服务器
- **流量统计**: 按规则/节点统计入站出站流量
- **限速控制**: 按规则设置速率限制 (KB/s)
- **用户权限**: 管理员/普通用户角色，JWT 认证
- **Web UI**: 内嵌精美深色主题管理界面
- **一键部署**: 提供安装脚本 + systemd 服务管理

## 🏗 架构

```
                    FlowGate Panel (面板端)
                    ┌──────────────────┐
                    │  Web UI + API    │
                    │  SQLite + WSHub  │
                    └────────┬─────────┘
                             │ WebSocket
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │  Node A  │  │  Node B  │  │  Node C  │
        │ TCP/UDP  │  │ TCP/UDP  │  │ TCP/UDP  │
        │ 转发引擎  │  │ 转发引擎  │  │ 转发引擎  │
        └──────────┘  └──────────┘  └──────────┘
```

## 🔐 Phase 1 隧道引擎

Phase 1 已经为 TCP 规则补上基础隧道能力，包括：

- TLS 隧道
- WebSocket 隧道
- 连接池预热
- 协议检测与阻断
- PROXY Protocol

完整字段说明、限制条件、示例和验收清单见：

- [docs/phase1-tunnel-engine.md](docs/phase1-tunnel-engine.md)

当前限制：

- `WS + 入站 TLS` 当前禁止
- `WSS` 当前不支持
- `UDP` 不支持 tunnel 能力

### Smoke 验证

基础 smoke：

```bash
python3 scripts/smoke_test.py --binary ./.tmp/flowgate
```

Phase 1 smoke：

```bash
python3 scripts/smoke_test.py --binary ./.tmp/flowgate --phase1
```

Windows 补充说明：

- 本地若没有 C 编译器（例如 `gcc` / `clang`），请传 `--binary`
- 或改在 Linux 上运行 smoke，避免 SQLite 临时构建失败

## 🚀 快速开始

### 1. 编译

```bash
# 安装依赖
go mod tidy

# 编译 Linux amd64
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o flowgate -ldflags '-s -w' ./cmd/flowgate/
```

> 需要 Go 1.21+ 和 CGO (SQLite 依赖)

### 2. 部署面板端

```bash
# 上传到服务器后
chmod +x flowgate
./flowgate panel --port 8080
```

或使用安装脚本：

```bash
bash scripts/install.sh --port 8080
```

### 3. 部署节点端

在面板中创建节点后，复制部署命令到目标服务器执行：

```bash
./flowgate node --panel ws://panel-ip:8080/ws/node --key <YOUR_API_KEY>
```

或使用安装脚本：

```bash
bash scripts/install_node.sh --panel ws://panel-ip:8080/ws/node --key <YOUR_API_KEY>
```

### 4. 访问面板

浏览器打开 `http://your-server-ip:8080`，首次访问注册管理员账号。

## 📋 命令参考

```
flowgate panel [options]    # 启动面板模式
  --host     监听地址 (默认: 0.0.0.0)
  --port     监听端口 (默认: 8080)
  --db       数据库路径 (默认: flowgate.db)
  --secret   JWT 密钥
  --tls      启用 TLS
  --cert     TLS 证书文件
  --key      TLS 密钥文件

flowgate node [options]     # 启动节点模式
  --panel    面板 WebSocket 地址 (必填)
  --key      节点 API 密钥 (必填)
  --tls      使用 wss://
```

## 🛠 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go + Gin |
| 数据库 | SQLite |
| 前端 | Vanilla JS (内嵌) |
| 通信 | WebSocket |
| 认证 | JWT + bcrypt |

## 📄 License

MIT License
