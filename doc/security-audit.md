# Sun-Panel 安全审查

范围为 fork 基线 `59831ce`，通过静态代码审查确认风险和触发条件。建议按编号选择整改项。新增排序接口已独立校验组与卡片的归属，新增动态口令流程已加账号级尝试限制和防重放。

| 编号 | 优先级 | 风险和证据 | 建议 |
| --- | --- | --- | --- |
| S1 | 高 | 登录入口缺少针对密码尝试的限速，首启密码是固定值。图形验证码依赖版本命中 CVE-2023-45292。密码用无盐 MD5 保存。见 `service/api/api_v1/system/login.go`、`service/initialize/database/connect.go`、`service/go.mod`、`service/lib/cmn/base.go`。 | 强制首次改密，按来源和账号限速，更新验证码依赖。成功登录时平移旧哈希到 Argon2id。 |
| S2 | 高 | 登录用户可调用服务端图片代理（SSRF 风险），请求缺少目的地址限制。图标获取功能也需要检查内部地址和重定向。见 `service/api/api_v1/panel/itemIcon.go`、`service/lib/siteFavicon/`。 | 下线未使用的代理，或在连接时限制目标地址及重定向，并设超时和响应大小上限。 |
| S3 | 高（多账号） | 旧编辑接口通过提交对象 ID 更新现有卡片或组，更新条件缺少当前用户限制。见 `service/biz/item.go`、`service/biz/itemGroup.go` 与对应 API。 | 以记录 ID 和当前用户 ID 共同约束更新，同时验证目标组归属。 |
| S4 | 高（多账号） | 上传允许 SVG，远程图标分支的类型校验不一致。文件通过面板同源静态路由提供，浏览器直接打开主动内容时有跨站脚本（XSS）风险。见 `service/api/api_v1/system/file.go`、`service/api/api_v1/panel/itemIcon.go`、`service/router/A_ENTER.go`。 | 校验实际文件类型，将主动内容与面板来源隔离，并保护旧文件路径。 |
| S5 | 中 | 开启公开模式后，访客可用的数据可能包括内网链接和监控信息。见 `service/api/api_v1/middleware/PublicModeInterceptor.go`、`service/router/panel/itemIcon.go`、`service/router/system/monitor.go`。 | 公开响应使用字段白名单，单独控制监控可见性。 |
| S6 | 中 | 用户模型包含敏感字段，部分调用直接序列化模型。浏览器保存会话令牌的前端密钥可从源码获得。见 `service/models/User.go`、`service/api/api_v1/panel/`、`src/utils/storage/`。 | 明确响应字段，审查会话生命周期，并制定令牌存储迁移方案。 |
| S7 | 中（视部署） | 容器默认以 root 运行。挂载 `docker.sock` 将增加面板或依赖遭入侵时的宿主机风险。见 `Dockerfile`、`service/router/docker/`。 | 默认非 root。需要 Docker 状态时使用最小权限代理。 |
| S8 | 运维与隐私 | 现有 PRO 模块在后台连接第三方授权服务。见 `service/biz/proAuth.go`。 | 明确出站用途和可配置项。 |

依赖是否命中安全公告和运行时可达性是两项不同的检查。建议后续结合最终镜像与前后端锁文件复查实际运行依赖。部署时请优先处理 S1–S4，尤其是面板开放公网登录或允许多人使用时。
