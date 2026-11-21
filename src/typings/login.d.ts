declare namespace Login{

    interface LoginReqest{
        username:string 
        password:string
        vcode?:string
        // 已启用 TOTP 时必填
        totpCode?:string
    }

	interface LoginResponse extends User.Info{
		token :string
        mustChangePassword?: boolean
	}

    interface ResetPasswordByVCodeReqest extends System.Register.SendRegisterVcodeRquest{
    }

}