# OAuth Model Policy

`oauth-model-policy` 是 CLIProxyAPI Pro 的动态库插件，用于按 OAuth 账号的套餐，从该账号原本可用的模型集合中减去模型。首个实现仅支持 xAI；Core 暴露的是通用 `AuthModelFilter` 能力，不包含任何 xAI 套餐知识。

## 生效顺序

账号模型注册按以下顺序处理：

1. upstream 全局或逐账号 `excluded_models`
2. `oauth-model-policy` 套餐规则
3. OAuth 模型 alias 与账号 prefix
4. 写入模型注册表

插件只能返回需要排除的现有模型 ID，不能增加模型或修改模型元数据。最终模型注册表同时决定 `/v1/models` 的聚合结果和请求调度时的账号候选集合。

## xAI 套餐

支持以下规则键：

- `free`
- `supergrok`：月限额 15000 cents
- `x-premium-plus`：月限额 20000 cents
- `supergrok-heavy`：月限额 150000 cents
- `paid-unknown`：已付费但限额未识别
- `_unknown`：套餐无法解析时的回退规则
- `_default`：没有更具体规则时的最终回退规则

插件先从 auth metadata、attributes 和 storage 中读取 `plan_type`、`planType`、`plan` 或 `package`。若没有本地套餐信息，会通过 Core 的受控 HTTP callback 请求 xAI billing API。解析成功的结果按账号缓存；临时探测失败时优先使用过期缓存，最后匹配 `_unknown`。正常的 billing 探测失败不会中断模型注册。

## 配置示例

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    oauth-model-policy:
      enabled: true
      priority: 10
      cache-ttl: 30m
      resolve-timeout: 15s
      providers:
        xai:
          plans:
            free:
              excluded-models:
                - "grok-pro-*"
            supergrok:
              excluded-models:
                - "grok-4.5-*"
            x-premium-plus:
              excluded-models: []
            supergrok-heavy:
              excluded-models: []
            paid-unknown:
              excluded-models:
                - "grok-experimental-*"
            _unknown:
              excluded-models:
                - "grok-pro-*"
```

`excluded-models` 使用区分大小写无关的 Go `path.Match` 通配规则。模型 ID 通常不包含 `/`，可使用 `*`、`?` 和字符集合。

## xAI 套餐探测

当本地没有套餐信息时，插件请求：

```text
GET https://cli-chat-proxy.grok.com/v1/billing
Authorization: Bearer <access_token>
x-xai-token-auth: xai-grok-cli
x-grok-client-version: 0.2.91
x-userid: <optional user id>
```

access token 与可选 user ID 仍由 Core 持有，插件通过绑定当前 auth 的 Host HTTP callback 发起请求，不自行创建绕过 Core transport policy 的网络客户端。

## 构建

```bash
go test ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o oauth-model-policy.so .
```

发布产物会按 `plugins/<goos>/<goarch>/oauth-model-policy.<ext>` 打包。Windows ARM64、FreeBSD 和 `_no-plugin` 产物暂不内置动态插件。
