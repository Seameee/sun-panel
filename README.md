# Sun-Panel

Sun-Panel 是一个自托管导航面板，可以分组管理网站和服务，为同一张卡片设置内外网地址，也可以用作浏览器首页。这个分支以官方 [v1.3.0](https://github.com/hslr-s/sun-panel/tree/v1.3.0)（`31fd128`）为基础，沿用 [MIT 许可](LICENSE)。

## v1.3.1-preview.1

- 在「个人信息 → 账号安全」绑定动态口令（TOTP）或通行密钥（Passkey）。开启 TOTP 后，密码登录需要输入验证器中的动态口令。已注册的通行密钥可以直接登录。
- 登录后点击右下角的铅笔开始排序。卡片可以在组内排序，也可以拖到其他组。点击勾号保存，点击叉号取消。
- 旧密码在登录成功后改用 Argon2id 存储。密码登录按账号和来源限速。初始管理员使用原版默认密码时，首次登录会要求改密。
- 卡片和分组的写入会检查归属。自动获取站点图标时只访问公网 HTTP(S) 地址，内网服务可以手动上传图标。公开首页隐藏内网地址和服务器硬件详情。上传图片会校验内容，旧文件仍可通过原路径访问。
- 更新了 Go、Vue、Vite 和运行时依赖。容器使用 UID/GID `10001:10001` 运行。

## Docker 安装

```sh
docker compose up -d
```

仓库的 `docker-compose.yml` 使用 GHCR 预览镜像。本机访问 `http://127.0.0.1:3002`。首次启动后运行下面的命令，为 `admin@sun.cc` 生成密码。也可以在首次启动前用 `SUN_PANEL_INITIAL_PASSWORD` 提供密码。

```sh
docker compose exec sun-panel /app/sun-panel -password-reset
```

从 HTTPS 访问面板时，在 `conf/conf.ini` 加入下面的配置并重启容器。`webauthn_origins` 填浏览器地址栏里的来源，使用非默认端口时也写上端口。注册后请保持 `webauthn_rp_id` 稳定。

```ini
[security]
webauthn_rp_id=panel.example.com
webauthn_origins=https://panel.example.com
```

## 从 1.3.0 升级

1. 停止旧容器，备份 `conf`、`database` 和 `uploads`，同时保存原 Compose 文件。使用 MySQL 时单独备份数据库。使用自定义上传目录时，备份配置里 `source_path` 指向的目录。
2. 在备份副本上试跑新版。沿用原路径和连接配置。使用绑定挂载时，给容器用户 `10001:10001` 授予挂载目录所需的权限。也可在 Compose 中设置已有的非 root UID/GID。
3. 用原账号登录，检查分组、卡片、图片和自定义设置。数据库会在启动时平移。新版登录后生成的 Argon2id 密码无法由旧版验证。需要回退时，使用原 Compose 文件与升级前的数据库和文件快照。

从先前的 `v1.7.0-preview.1` 迁移时，原来的导航卡片和认证记录可沿用。该版本特有的 Docker 卡片、内置应用及高级卡片样式在本分支中没有对应功能。请保存旧数据库快照以备查。若用户启用了 TOTP，请备份验证器。数据库中保存 TOTP 的共享密钥，应限制备份的访问权限。

## 开发和构建

前端使用 Node.js 24 和 `pnpm`。后端使用带 C 编译器的 Go 工具链，以便构建 SQLite 驱动。

```sh
pnpm install --frozen-lockfile
pnpm run type-check
VITE_GLOB_API_URL=/api pnpm run build-only
```

推送 `v*-preview.*` 标签时，GitHub Actions 构建并验证 `linux/amd64` 容器，镜像以同名标签发布到 GHCR。构建和镜像可以在仓库的 [Actions](https://github.com/Seameee/sun-panel/actions/workflows/docker-preview.yml) 页面查询。
