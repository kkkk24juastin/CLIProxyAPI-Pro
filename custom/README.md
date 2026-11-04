# 自定义：面板显示渠道名称

本 fork 只在 ssfun/CLIProxyAPI-Pro 的发布产物上叠加一个面板补丁，后端二进制保持不变。

## 补丁内容（`apply_channel_name.py`）

1. `services/api/transformers.ts`：v8 配置里每个 Key 的字段是 `auth_index`，原解析只读 `auth-index`，导致请求日志无法对应回渠道。改为两种写法都读。
2. `pro/modules/monitoring/features/sourceResolver.ts`：API Key 类渠道在监控/实时日志中显示 v8 分组的 `name`（设置了 `prefix` 时仍优先显示 prefix）。
3. `features/providers/components/ProviderResourceTable.tsx`：渠道列表主标题显示分组 `name`，Key 预览移到副标题。

每处替换都校验命中次数，上游代码变动时构建会直接失败，不会静默生成不同的面板。

## 构建（`.github/workflows/custom-image.yml`）

每 6 小时检查 ssfun/CLIProxyAPI-Pro 最新 Release，按其中记录的 Management 与定制 commit 精确复现，叠加本补丁后：

- 发布 `management.html` 到本仓库 Release（供 CPA 自动更新面板，`management.panel-github-repository` 指向本仓库）
- 推送镜像 `ghcr.io/kkkk24juastin/cliproxyapi-pro:{latest,<上游tag>}`，即 `sfun/cliproxyapi-pro:<上游tag>` 仅替换面板文件

原仓库的 CI / 发布流程已从本 fork 移除（它们依赖原作者的 Docker Hub 密钥）。
