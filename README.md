# Mihomo Stack

Mihomo Stack 是面向单节点的完整 Mihomo 容器部署栈。它集成 Mihomo、MetaCubeXD、
可选 subconverter，以及名为 `mihomo-guardian` 的安全配置控制面。Guardian 把订阅
下载、多订阅合并、Mihomo 原生校验、原子切换、健康检查和自动回滚组成一个可审计的
更新事务。

默认部署同时提供：

- Mihomo 代理内核和 MetaCubeXD 面板；
- Go 编写的 Guardian API、调度器与 CLI；
- Vue 3 管理界面；
- amd64、arm64、arm/v7 镜像；
- secret 文件、非 root、只读 rootfs 和最小权限容器配置；
- 固定上游源码提交、checksum 校验、CI 测试、SBOM、漏洞扫描和镜像签名流程。

> 请在遵守所在地法律、网络服务条款和订阅提供方规则的前提下使用。本项目不提供节点、
> 订阅或规避访问控制的服务。

## 为什么不是一组 Shell 脚本

配置更新可能直接导致整台设备失去代理连接。Guardian 把它作为事务处理：

```text
秘密文件中的订阅 URL
        │
        ▼
下载 / 可选转换 / 确定性合并
        │
        ▼
强制应用容器运行时设置
        │
        ▼
临时文件 ──► mihomo -t 原生校验
        │
        ▼
备份旧配置 ──► 原子替换 ──► Controller reload ──► 健康检查
                                  │                    │
                                  └──── 失败自动回滚 ◄─┘
```

任何下载、解析或验证错误都不会触碰当前配置。更新期间只允许一个事务运行。

## 快速开始

依赖 Docker Engine 24+ 和 Docker Compose v2。仓库根目录执行：

```bash
cp deploy/.env.example deploy/.env
openssl rand -hex 32 > secrets/guardian_admin_token.txt
openssl rand -hex 32 > secrets/mihomo_controller_secret.txt
printf '%s\n' 'https://example.com/your-subscription' > secrets/primary_subscription_url.txt
chmod 600 secrets/*.txt

docker compose -f deploy/compose.yaml up -d --build
```

启动后：

- Guardian：<http://localhost:8080>
- MetaCubeXD：<http://localhost:9090/ui>
- Mixed proxy：`localhost:7890`

登录令牌是 `secrets/guardian_admin_token.txt` 的内容。浏览器只在登录请求中发送一次，
之后使用 HttpOnly、SameSite=Strict 签名会话 Cookie，不写入 localStorage。

`9090` 默认只绑定宿主回环地址。`7890` 默认监听所有宿主网卡；如果只供本机使用，请在
`deploy/.env` 设置：

```dotenv
PROXY_BIND_ADDRESS=127.0.0.1
```

## 日常操作

```bash
# 状态和日志
docker compose -f deploy/compose.yaml ps
docker compose -f deploy/compose.yaml logs -f guardian mihomo

# 停止服务（保留配置和备份）
docker compose -f deploy/compose.yaml down

# 同时删除数据卷；这会删除配置、历史和备份
docker compose -f deploy/compose.yaml down -v
```

Guardian CLI 也可单独运行：

```bash
mihomo-guardian doctor -config config.yaml
mihomo-guardian validate -config config.yaml
mihomo-guardian update -config config.yaml
```

## 配置

完整示例见 [`config.example.yaml`](config.example.yaml)。关键段落：

- `auth`：只接受管理员 token 文件，不接受 YAML 明文 token。
- `subscription.sources`：每个订阅只保存 `url_file` 路径。
- `mihomo`：Controller 地址、校验二进制及强制运行时设置。
- `update`：启动更新、固定间隔和健康检查延迟。
- `storage`：活动配置、备份数量和历史数量。
- `converter`：可选的本地 subconverter 兼容接口。

修改默认容器配置时，复制示例并在 Compose 的 `guardian.volumes` 增加只读挂载：

```yaml
- ./config.yaml:/app/config.yaml:ro
```

相对路径以配置文件目录为基准。容器内 Guardian 与 Mihomo 必须以完全相同的路径挂载
活动配置；参考 Compose 统一使用 `/data/config.yaml`。

### 多订阅

多个 Clash/Mihomo YAML 订阅会在本地确定性合并。重名但内容不同的代理、provider 或
代理组会让更新失败，避免静默覆盖。如果输入不是 Clash YAML，可自行部署
subconverter，将 `converter.enabled` 设为 `true`，并把 `converter.api_url` 指向该
私有服务。仓库提供了可选容器：

```bash
docker compose -f deploy/compose.yaml --profile converter up -d --build
```

默认配置仍关闭转换，启用前还需挂载修改后的 `config.yaml`。默认拒绝公网 converter，
避免把秘密订阅 URL 交给第三方。

### Runtime enforcement

容器部署默认强制覆盖以下顶层字段：

- `mixed-port`
- `allow-lan`
- `external-controller`
- `external-ui`
- `secret`

这避免订阅刷新后 Controller 或映射端口突然失效。高级的非容器部署可以关闭
`mihomo.enforce_runtime_settings`，随后需自行保证 Controller 始终可达。

## 安全模型

- 所有 URL 和 token 从文件读取，API 和日志不返回其内容。
- 容器入口只在启动时读取宿主 `0600` secret，将其复制到 tmpfs 后立即降权；Guardian
  和 Mihomo 业务进程以 UID 10001 运行。
- 跨主机重定向会移除自定义请求头。
- HTTPS 始终验证证书，不提供 insecure 开关。
- 候选和状态文件使用 `0600`，通过同目录临时文件原子替换。
- Cookie 写操作必须通过同源检查；CLI 可以使用 Bearer token。
- Controller 端口只映射到宿主回环地址。
- Mihomo 从固定版本和 commit 的上游源码构建；其他上游制品固定版本并校验 SHA-256。

威胁边界、漏洞报告方式见 [`SECURITY.md`](SECURITY.md)。不要把 secret、订阅 URL 或
完整配置粘贴到公开 Issue。

## 本地开发

需要 Go 1.24+、Node.js 22+：

```bash
go test ./...
go test -race ./...
go vet ./...

cd web
npm ci
npm run typecheck
npm run test
npm run build
```

本地启动 API 时先准备 `config.yaml`、订阅 secret 和可用的 Mihomo 二进制。开发前请读
[`AGENTS.md`](AGENTS.md)；模块行为以 [`docs/specs`](docs/specs) 中的规范为准。

## 开源与第三方组件

Mihomo Stack 自有代码使用 MIT License。发布镜像同时包含 GPL-3.0 的 Mihomo 和
MetaCubeXD，它们保留各自许可证。固定版本、源码链接、补丁说明和 checksum 见
[`THIRD_PARTY.md`](THIRD_PARTY.md)。
