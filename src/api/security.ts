import { post } from '@/utils/request'

/**
 * 账号安全相关接口（TOTP / Passkey）
 * 请求体可能包含密码与动态口令，统一开启 maskErrorLog，
 * 避免网络异常时把请求体（含密码、一次性口令）写进控制台。
 */

// 安全状态：TOTP 是否启用、已注册 Passkey、当前环境是否支持 Passkey
export function getSecurityStatus<T>() {
  return post<T>({
    url: '/user/security/status',
  })
}

// 开始绑定 TOTP，返回流程 id、密钥与 otpauth 链接
export function securityTotpBegin<T>(password: string) {
  return post<T>({
    url: '/user/security/totp/begin',
    data: { password },
    maskErrorLog: true,
  })
}

// 提交动态码完成 TOTP 绑定
export function securityTotpFinish<T>(flowId: string, code: string) {
  return post<T>({
    url: '/user/security/totp/finish',
    data: { flowId, code },
    maskErrorLog: true,
  })
}

// 关闭 TOTP
export function securityTotpDisable<T>(password: string, code: string) {
  return post<T>({
    url: '/user/security/totp/disable',
    data: { password, code },
    maskErrorLog: true,
  })
}

// 开始注册 Passkey
export function securityPasskeyBegin<T>(password: string, name: string, totpCode?: string) {
  return post<T>({
    url: '/user/security/passkey/begin',
    data: { password, name, totpCode },
    maskErrorLog: true,
  })
}

// 提交注册凭据
export function securityPasskeyFinish<T>(flowId: string, credential: Security.CredentialJson) {
  return post<T>({
    url: '/user/security/passkey/finish',
    data: { flowId, credential },
    maskErrorLog: true,
  })
}

// 删除已注册的 Passkey
export function securityPasskeyDelete<T>(id: string | number, password: string, totpCode?: string) {
  return post<T>({
    url: '/user/security/passkey/delete',
    data: { id, password, totpCode },
    maskErrorLog: true,
  })
}

// 游客 Passkey 登录：开始（discoverable，无用户名）
export function passkeyLoginBegin<T>() {
  return post<T>({
    url: '/login/passkey/begin',
    maskErrorLog: true,
  })
}

// 游客 Passkey 登录：提交凭据，返回与 /login 相同的 UserInfo
export function passkeyLoginFinish<T>(flowId: string, credential: Security.CredentialJson) {
  return post<T>({
    url: '/login/passkey/finish',
    data: { flowId, credential },
    maskErrorLog: true,
  })
}
