# Sun-Panel

Sun-Panel 是一个自托管导航面板，可以分组管理网站和服务，为同一张卡片设置内外网地址，也可以用作浏览器首页。这个分支以官方 [v1.3.0](https://github.com/hslr-s/sun-panel/tree/v1.3.0)（`31fd128`）为基础，沿用 [MIT 许可](LICENSE)。

## v1.3.1

- 动态验证码输入框支持密码管理器的 `one-time-code` 自动填充。Passkey 创建和删除流程中的动态验证码也使用相同的标准字段。
- Passkey 创建时间按本地时间显示。
- 稳定版镜像发布到 GHCR，并同步更新 `latest` 标签。

## v1.3.1-preview.4

- Passkey 支持通过 `SUN_PANEL_WEBAUTHN_ORIGIN` 配置。Compose 显式使用 UID/GID `10001:10001`。
- 密码登录和 Passkey 验证限制请求体大小。Passkey 登录发起按来源限流，挑战池满时保留已发出的有效挑战。
- 公开展示账号停用后，访客无法继续查看该账号的面板。

## v1.3.1-preview.3

- 右下角排序的确认和取消按钮采用与工具栏一致的白色图标。
- 账号安全卡片在应用面板内滚动。Passkey 入口会提示站点配置、HTTPS 或浏览器支持问题。
- 登录表单支持密码管理器识别用户名、密码和一次性验证码。在密码管理器中为该站点保存 TOTP 后，面板要求验证码时可调用管理器自动填充。

问题和建议可提交到 [GitHub Issues](https://github.com/Seameee/sun-panel/issues)。

## v1.3.1-preview.2

- 在「个人信息 → 账号安全」绑定动态口令（TOTP）或通行密钥（Passkey）。开启 TOTP 后，密码登录需要输入验证器中的动态口令。已注册的通行密钥可以直接登录。
- 登录后点击右下角的铅笔开始排序。卡片可以在组内排序，也可以拖到其他组。点击勾号保存，点击叉号取消。
- 旧密码在登录成功后改用 Argon2id 存储。密码登录按账号和来源限速。初始管理员使用原版默认密码时，首次登录会要求改密。
- 卡片和分组的写入会检查归属。自动获取站点图标时只访问公网 HTTP(S) 地址，内网服务可以手动上传图标。公开首页隐藏内网地址和服务器硬件详情。上传图片会校验内容，旧文件仍可通过原路径访问。
- 更新了 Go、Vue、Vite 和运行时依赖。容器使用 UID/GID `10001:10001` 运行。

## Docker 安装

```sh
docker compose up -d
```

仓库的 `docker-compose.yml` 使用 GHCR 预览镜像，并以 UID/GID `10001:10001` 运行。本机访问 `http://127.0.0.1:3002`。首次启动后运行下面的命令，为 `admin@sun.cc` 生成密码。也可以在首次启动前用 `SUN_PANEL_INITIAL_PASSWORD` 提供密码。

```sh
docker compose exec sun-panel /app/sun-panel -password-reset
```

### 配置 Passkey

将浏览器实际访问的 HTTPS 来源传给容器。使用非默认端口时带上端口，例如 `https://panel.example.com:8443`。修改环境变量后，用 Compose 重新创建容器。

```sh
SUN_PANEL_WEBAUTHN_ORIGIN=https://panel.example.com docker compose up -d
```

程序用完整来源配置 `webauthn_origins`，并从中提取主机名作为 `webauthn_rp_id`。环境变量有值时，两项一起覆盖 INI 配置。留空时沿用 `conf/conf.ini` 中 `[security]` 的配置，支持原有的多来源设置和父域 RP ID。已有通行密钥的站点应保持原 RP ID。如果原 RP ID 是父域，请继续使用 INI 配置。

## 从 1.3.0 升级

1. 停止旧容器，备份 `conf`、`database` 和 `uploads`，同时保存原 Compose 文件。使用 MySQL 时单独备份数据库。使用自定义上传目录时，备份配置里 `source_path` 指向的目录。
2. 在备份副本上试跑新版。沿用原路径和连接配置。使用绑定挂载时，给容器用户 `10001:10001` 授予挂载目录所需的权限。也可在 Compose 中设置已有的非 root UID/GID。
3. 用原账号登录，检查分组、卡片、图片和自定义设置。数据库会在启动时平移。新版登录后生成的 Argon2id 密码无法由旧版验证。需要回退时，使用原 Compose 文件与升级前的数据库和文件快照。

启用了 TOTP 的用户应备份验证器。数据库中保存 TOTP 的共享密钥，请限制备份的访问权限。

## 开发和构建

前端使用 Node.js 24 和 `pnpm`。后端使用带 C 编译器的 Go 工具链，以便构建 SQLite 驱动。

```sh
pnpm install --frozen-lockfile
pnpm run type-check
VITE_GLOB_API_URL=/api pnpm run build-only
```

推送 `v*-preview.*` 标签时，GitHub Actions 构建并验证 `linux/amd64` 容器，镜像以同名标签发布到 GHCR。构建和镜像可以在仓库的 [Actions](https://github.com/Seameee/sun-panel/actions/workflows/docker-preview.yml) 页面查询。
