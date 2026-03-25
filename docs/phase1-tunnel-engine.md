# FlowGate Phase 1 隧道引擎指南

FlowGate 的 Phase 1 目标是让 TCP 转发不再“裸奔”，在不引入 Docker、MySQL、Redis 的前提下，为现有规则补上基础隧道能力。

当前 Phase 1 已提供以下能力：

- TLS 隧道
- WebSocket 隧道
- 连接池预热
- 协议检测与阻断
- PROXY Protocol

这套能力只作用于 `TCP` 规则，`UDP` 和 `TCP+UDP` 中的 UDP 部分不支持 tunnel 特性。

## 配置字段

以下字段已经进入 `Rule` / `RuleConfig`：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `proxy_protocol` | `int` | `0` | `0=关闭`，`1=v1`，`2=v2` |
| `blocked_protos` | `string` | `""` | 逗号分隔的阻断列表，例如 `socks,http` |
| `pool_size` | `int` | `0` | 连接池大小，`0=关闭` |
| `tls_mode` | `string` | `"none"` | `none / client / server / both` |
| `tls_sni` | `string` | `""` | 出站 TLS 使用的 SNI |
| `ws_enabled` | `bool` | `false` | 是否启用 WebSocket 隧道监听 |
| `ws_path` | `string` | `"/ws"` | WebSocket 路径 |

字段行为补充：

- `blocked_protos` 当前识别 `socks`、`http`、`tls`
- `socks` 会同时匹配 `socks4` 和 `socks5`
- `tls_sni` 只在 `tls_mode=server` 或 `tls_mode=both` 时有意义
- 旧规则不填写这些字段时，行为保持不变

## 支持矩阵

| 场景 | 当前状态 | 说明 |
|------|----------|------|
| `tls_mode=none` | 支持 | 不启用 TLS |
| `tls_mode=client` | 支持 | 入站 TLS，客户端连到 FlowGate 时走 TLS |
| `tls_mode=server` | 支持 | 出站 TLS，FlowGate 连目标端时走 TLS |
| `tls_mode=both` | 支持 | 同时启用入站和出站 TLS |
| `ws_enabled=true` | 支持 | 规则监听端改为 WebSocket 接入 |
| `ws_enabled=true` + `tls_mode=client` | 禁止 | 前端和 API 都会拒绝 |
| `ws_enabled=true` + `tls_mode=both` | 禁止 | 前端和 API 都会拒绝 |
| `WSS` | 不支持 | 当前没有独立的 WSS 模式 |
| `UDP` tunnel | 不支持 | Phase 1 仅覆盖 TCP |

当前限制的核心原因是：Phase 1 只支持 `WS`，不支持 `WSS`。因此 `WebSocket + 入站 TLS` 组合在本期被明确禁止，而不是以半成品状态放出。

## 典型用法

### 1. WS-only

适合需要穿 CDN、HTTP 反代或防火墙环境的场景。

```json
{
  "protocol": "tcp",
  "ws_enabled": true,
  "ws_path": "/ws",
  "tls_mode": "none"
}
```

预期行为：

- 规则监听端口接受 `ws://host:port/ws`
- 目标端仍然使用普通 TCP 连接

### 2. 入站 TLS

适合让客户端直接以 TLS 方式连到 FlowGate。

```json
{
  "protocol": "tcp",
  "tls_mode": "client",
  "ws_enabled": false
}
```

预期行为：

- FlowGate 在监听端口做 TLS 握手
- 目标端仍然使用普通 TCP

### 3. 出站 TLS

适合目标端本身要求 TLS，例如后端服务只接受 TLS 流量。

```json
{
  "protocol": "tcp",
  "tls_mode": "server",
  "tls_sni": "example.com"
}
```

预期行为：

- 客户端到 FlowGate 仍是普通 TCP
- FlowGate 到目标端改为 TLS

### 4. PROXY Protocol v1 / v2

适合后端需要拿到真实客户端 IP 的场景。

```json
{
  "protocol": "tcp",
  "proxy_protocol": 1
}
```

或：

```json
{
  "protocol": "tcp",
  "proxy_protocol": 2
}
```

预期行为：

- `v1` 写入文本头
- `v2` 写入二进制头

### 5. HTTP 阻断

适合拒绝明显的 HTTP 探测流量。

```json
{
  "protocol": "tcp",
  "blocked_protos": "http"
}
```

预期行为：

- 首包识别为 HTTP 时，连接直接丢弃
- 后端目标不应收到这次连接

### 6. 连接池预热

适合减少首次请求的建连延迟。

```json
{
  "protocol": "tcp",
  "pool_size": 2
}
```

预期行为：

- 规则启动后会预建连接
- 实际转发时优先复用池中连接

## 验收清单

### 自动验收

1. 基础 smoke

```bash
python3 scripts/smoke_test.py --binary ./.tmp/flowgate
```

预期结果：

- Panel 可启动
- Node 可注册上线
- 普通 TCP 规则可创建并转发成功

2. Phase 1 smoke

```bash
python3 scripts/smoke_test.py --binary ./.tmp/flowgate --phase1
```

预期结果：

- 基础 smoke 全部通过
- `WS + client TLS` 创建请求返回 400
- `tls_mode=client` 场景可成功完成 TLS 入站 roundtrip
- `tls_mode=server` 场景可成功完成 TLS 出站 roundtrip
- `ws_enabled=true` 场景可成功完成 WebSocket roundtrip
- `blocked_protos=http` 时目标端不会收到连接
- `proxy_protocol=1/2` 时目标端能收到对应头
- `pool_size=2` 时预热成功且转发仍可正常工作

Windows 补充说明：

- 如果本机没有 `gcc` 或 `clang`，不要让脚本在本地临时构建 SQLite 二进制
- 这时应传 `--binary` 使用预编译二进制，或改在 Linux 上运行 smoke

### 人工验收

1. 创建普通 TCP 规则
   预期：行为与 Phase 1 之前一致，转发成功
2. 创建 `WS-only` 规则
   预期：`ws://<host>:<port><ws_path>` 可完成转发
3. 创建 `tls_mode=server` 规则
   预期：后端只接受 TLS 时仍可正常工作
4. 创建 `proxy_protocol=1` 和 `proxy_protocol=2` 规则
   预期：后端可解析到真实客户端地址
5. 创建 `pool_size>0` 规则
   预期：规则启动后预热连接，实际转发不报错
6. 创建 `blocked_protos=http` 规则
   预期：HTTP 探测流量被拒绝，目标端无请求日志
7. 尝试创建 `ws_enabled=true` 且 `tls_mode=client/both` 的规则
   预期：前端阻止提交；若绕过前端，API 返回 400

## 兼容性结论

- 旧规则默认零行为变化
- tunnel 字段默认都是关闭态
- 当前 Phase 1 可以作为 TCP 规则的增强层使用，但还不是完整的多跳/端口复用架构
- Phase 2 再解决节点分组、多跳链路、负载均衡和端口复用
