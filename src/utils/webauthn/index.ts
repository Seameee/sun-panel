/**
 * 浏览器 WebAuthn 辅助函数
 * 后端以 JSON 下发/接收选项，二进制字段用 base64url 字符串表示，这里负责解析与序列化。
 */

export type WebAuthnSupport = 'ok' | 'insecure' | 'unsupported'
export type WebAuthnFailure = 'cancelled' | 'exists' | 'unsupported' | 'unknown'

/**
 * 检测当前环境的 WebAuthn 可用性
 * insecure 表示浏览器支持，但当前不是安全上下文（非 HTTPS / 非 localhost）
 */
export function getWebAuthnSupport(): WebAuthnSupport {
  if (typeof window === 'undefined' || typeof navigator === 'undefined')
    return 'unsupported'
  if (!window.isSecureContext)
    return 'insecure'
  if (typeof window.PublicKeyCredential === 'undefined' || !navigator.credentials)
    return 'unsupported'
  return 'ok'
}

/** base64url -> Uint8Array */
export function base64UrlToBytes(value: string): Uint8Array<ArrayBuffer> {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++)
    bytes[i] = binary.charCodeAt(i)
  return bytes
}

/** ArrayBuffer / Uint8Array -> base64url */
export function bytesToBase64Url(value: ArrayBuffer | Uint8Array | null | undefined): string {
  if (!value)
    return ''
  const bytes = value instanceof Uint8Array ? value : new Uint8Array(value)
  let binary = ''
  const chunkSize = 0x4000
  for (let i = 0; i < bytes.length; i += chunkSize)
    binary += String.fromCharCode(...Array.from(bytes.subarray(i, i + chunkSize)))
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/** 后端可能下发对象或 JSON 字符串 */
export function parsePublicKeyJson(publicKey: Security.PublicKeyJson | string | undefined | null): Security.PublicKeyJson {
  if (!publicKey)
    throw new Error('missing publicKey options')
  if (typeof publicKey === 'string')
    return JSON.parse(publicKey) as Security.PublicKeyJson
  return publicKey
}

function toDescriptors(list?: Security.CredentialDescriptorJson[]): PublicKeyCredentialDescriptor[] | undefined {
  if (!list || list.length === 0)
    return undefined
  return list.map(item => ({
    id: base64UrlToBytes(item.id),
    type: (item.type as PublicKeyCredentialType) || 'public-key',
    transports: item.transports as AuthenticatorTransport[] | undefined,
  }))
}

/** 解析注册选项 */
export function toCreationOptions(publicKey: Security.PublicKeyJson | string): PublicKeyCredentialCreationOptions {
  const json = parsePublicKeyJson(publicKey)
  if (!json.challenge || !json.user || !json.user.id)
    throw new Error('invalid creation options')

  return {
    challenge: base64UrlToBytes(json.challenge),
    rp: { id: json.rp?.id, name: json.rp?.name || document.title },
    user: {
      id: base64UrlToBytes(json.user.id),
      name: json.user.name,
      displayName: json.user.displayName || json.user.name,
    },
    pubKeyCredParams: (json.pubKeyCredParams as PublicKeyCredentialParameters[] | undefined) ?? [
      { type: 'public-key', alg: -7 },
      { type: 'public-key', alg: -257 },
    ],
    timeout: json.timeout,
    attestation: json.attestation as AttestationConveyancePreference | undefined,
    authenticatorSelection: json.authenticatorSelection as AuthenticatorSelectionCriteria | undefined,
    excludeCredentials: toDescriptors(json.excludeCredentials),
    extensions: json.extensions as AuthenticationExtensionsClientInputs | undefined,
  }
}

/** 解析登录（discoverable）选项 */
export function toRequestOptions(publicKey: Security.PublicKeyJson | string): PublicKeyCredentialRequestOptions {
  const json = parsePublicKeyJson(publicKey)
  if (!json.challenge)
    throw new Error('invalid request options')

  return {
    challenge: base64UrlToBytes(json.challenge),
    rpId: json.rpId,
    timeout: json.timeout,
    userVerification: (json.userVerification as UserVerificationRequirement | undefined) ?? 'preferred',
    allowCredentials: toDescriptors(json.allowCredentials),
    extensions: json.extensions as AuthenticationExtensionsClientInputs | undefined,
  }
}

/** 序列化注册凭据 */
export function serializeCreationCredential(credential: PublicKeyCredential): Security.CredentialJson {
  const response = credential.response as AuthenticatorAttestationResponse
  return {
    id: credential.id,
    rawId: bytesToBase64Url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? null,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: bytesToBase64Url(response.clientDataJSON),
      attestationObject: bytesToBase64Url(response.attestationObject),
      transports: typeof response.getTransports === 'function' ? response.getTransports() : undefined,
    },
  }
}

/** 序列化认证（登录）凭据 */
export function serializeAssertionCredential(credential: PublicKeyCredential): Security.CredentialJson {
  const response = credential.response as AuthenticatorAssertionResponse
  return {
    id: credential.id,
    rawId: bytesToBase64Url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? null,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: bytesToBase64Url(response.clientDataJSON),
      authenticatorData: bytesToBase64Url(response.authenticatorData),
      signature: bytesToBase64Url(response.signature),
      userHandle: response.userHandle ? bytesToBase64Url(response.userHandle) : null,
    },
  }
}

/** 把 WebAuthn 的异常归为便于提示的类别 */
export function classifyWebAuthnError(error: unknown): WebAuthnFailure {
  const name = (error as { name?: string } | null)?.name
  if (name === 'NotAllowedError' || name === 'AbortError')
    return 'cancelled'
  if (name === 'InvalidStateError')
    return 'exists'
  if (name === 'NotSupportedError' || name === 'SecurityError')
    return 'unsupported'
  return 'unknown'
}

/** 用户取消/超时等浏览器侧失败（没有业务错误码） */
export function isWebAuthnError(error: unknown): boolean {
  const name = (error as { name?: string } | null)?.name
  return name === 'NotAllowedError'
    || name === 'AbortError'
    || name === 'InvalidStateError'
    || name === 'NotSupportedError'
    || name === 'SecurityError'
}
