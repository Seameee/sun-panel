declare namespace Security{

	interface AuthenticatorSelectionJson{
		authenticatorAttachment?:string
		residentKey?:string
		requireResidentKey?:boolean
		userVerification?:string
	}

	// WebAuthn 选项的 JSON 形态：二进制字段均为 base64url 字符串
	interface PublicKeyJson{
		challenge:string
		rpId?:string
		timeout?:number
		attestation?:string
		userVerification?:string
		rp?:{ id?:string; name:string }
		user?:{ id:string; name:string; displayName?:string }
		pubKeyCredParams?:{ type:string; alg:number }[]
		authenticatorSelection?:AuthenticatorSelectionJson
		excludeCredentials?:CredentialDescriptorJson[]
		allowCredentials?:CredentialDescriptorJson[]
		extensions?:unknown
	}

	interface CredentialDescriptorJson{
		id:string
		type?:string
		transports?:string[]
	}

	// 服务端返回的“流程 + 选项”结构（begin 类接口）
	interface PublicKeyFlowResponse{
		flowId:string
		publicKey:PublicKeyJson | string
	}

	// TOTP 绑定开始（含密钥与 otpauth 链接）
	interface TotpBeginResponse{
		flowId:string
		secret:string
		otpauthUrl:string
	}

	// 已完成注册的 Passkey
	interface PasskeyInfo{
		id:string | number
		name:string
		createdAt:string
	}

	interface StatusResponse{
		totpEnabled:boolean
		passkeys:PasskeyInfo[]
		passkeyAvailable:boolean
	}

	interface CredentialResponseJson{
		clientDataJSON:string
		// 注册（create）
		attestationObject?:string
		transports?:string[]
		// 登录（get）
		authenticatorData?:string
		signature?:string
		userHandle?:string | null
	}

	// 发送给服务端的凭据（由浏览器凭据序列化而来）
	interface CredentialJson{
		id:string
		rawId:string
		type:string
		authenticatorAttachment?:string | null
		clientExtensionResults:unknown
		response:CredentialResponseJson
	}

}
