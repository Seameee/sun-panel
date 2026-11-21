# 从 Sun-Panel v1.3.0 升级

1. 停止旧实例，备份 `conf/`、`uploads/` 和 `database/`。使用外部 MySQL 时，另做数据库备份。使用自定义上传目录时，按 `source_path` 备份该目录。
2. 先在备份副本上运行新版。沿用原工作目录、配置文件和数据库连接。Docker 部署沿用原挂载路径 `/app/conf`、`/app/uploads`、`/app/database`。
3. 使用原账号登录，核对分组、链接顺序、站点设置和图片，再替换实际实例。新版启动时会自动迁移表结构。回滚时停止新版并恢复整份升级前的备份，然后启动旧版。

账号默认可以继续使用原密码登录。登录后在应用启动器进入「个人信息 → 账号安全」绑定动态口令（TOTP）或通行密钥（Passkey）。TOTP 二维码在浏览器本地生成。启用 TOTP 后，密码登录需要输入动态口令。已经启用的 TOTP 在新增或删除通行密钥时也需要验证。请保护数据库和备份，其中包含 TOTP 共享密钥。保留一个可用的备用通行密钥或验证器备份，以便设备丢失后登录。

## 启用通行密钥

在 `conf/conf.ini` 加入以下配置并重启。访问域名由 HTTPS 反向代理提供时，填写浏览器地址栏中的外部域名和端口。

```ini
[security]
webauthn_rp_id=panel.example.com
webauthn_origins=https://panel.example.com
```

`webauthn_origins` 支持逗号分隔的来源。每个来源由协议、主机名和非默认端口组成。建议用实际面板域名作为 `webauthn_rp_id`，以后保持不变。仅本机开发可使用 `localhost` 和 `http://localhost:3002`。认证挑战保存在进程内，多副本部署需保证同一个认证流程的两个请求进入同一实例。

## 排序

登录后点击首页右下角的铅笔按钮。卡片可在组内和组间拖动。点击勾号保存，点击叉号取消。

## 回归验证

`tests/upgrade_v1_3_0.py` 通过官方 `v1.3.0` 二进制创建独立测试数据，再用新版在同一份数据上启动两次。它检查账号、头像、分组、内外网链接、排序及上传图片。SQLite 默认使用临时目录。验证 MySQL 驱动时传入本机测试端口和空数据库名。测试数据库需要一个仅有该库权限的 `fixture` 用户。

```sh
python3 tests/upgrade_v1_3_0.py /path/to/sun-panel-1.3.0 /path/to/sun-panel-current
python3 tests/upgrade_v1_3_0.py /path/to/sun-panel-1.3.0 /path/to/sun-panel-current --uploads legacy-files
```
