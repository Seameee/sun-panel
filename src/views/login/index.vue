<script setup lang="ts">
import { NButton, NCard, NDivider, NForm, NFormItem, NInput, NSelect, NSwitch, useMessage, useOsTheme } from 'naive-ui'
import type { InputInst } from 'naive-ui'
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Captcha, SvgIcon } from '@/components/common'
import { login } from '@/api'
import { passkeyLoginBegin, passkeyLoginFinish } from '@/api/security'
import { useAppStore, useAuthStore, usePanelState } from '@/store'
import { router } from '@/router'
import { t } from '@/locales'
import { languageOptions } from '@/utils/defaultData'
import type { Language } from '@/store/modules/app/helper'
import { apiRespErrMsg } from '@/utils/request/apiMessage'
import { get } from '@/utils/request'
import { updatePassword } from '@/api/system/user'
import { classifyWebAuthnError, getWebAuthnSupport, serializeAssertionCredential, toRequestOptions } from '@/utils/webauthn'

const route = useRoute()
// const userStore = useUserStore()
const authStore = useAuthStore()
const appStore = useAppStore()
const panelState = usePanelState()
const osTheme = useOsTheme()
const ms = useMessage()
const loading = ref(false)
const defaultUsername = ref<string>(route.query.username as string || '')
const inputpasswordRef = ref<InputInst | null>(null)
const isDark = computed(() => {
  if (appStore.theme === 'auto')
    return osTheme.value === 'dark'
  else
    return appStore.theme === 'dark'
})
const languageValue = ref<Language>(appStore.language)
const captchaEnabled = ref(false)
const passkeyConfigured = ref(false)
const loginOptionsLoaded = ref(false)
const captchaRef = ref()

const form = ref<Login.LoginReqest>({
  username: defaultUsername.value,
  password: '',
})
const totpRequired = ref(false)
const passwordChangeToken = ref('')
const mustChangePassword = computed(() => passwordChangeToken.value !== '')
const newPassword = ref('')
const inputTotpRef = ref<HTMLInputElement | null>(null)
const passkeyLoading = ref(false)
const webAuthnSupport = getWebAuthnSupport()

// 登录成功后的统一处理（密码登录与 Passkey 登录共用）
function handleLoginSuccess(data: Login.LoginResponse) {
  if (data.mustChangePassword) {
    passwordChangeToken.value = data.token
    loading.value = false
    passkeyLoading.value = false
    return
  }
  authStore.setToken(data.token)
  form.value.password = ''
  form.value.totpCode = ''
  authStore.setUserInfo(data)
  panelState.resetPanelConfig() // 重置面板配置

  setTimeout(() => {
    ms.success(`Hi ${data.name},${t('login.welcomeMessage')}`)
    loading.value = false
    passkeyLoading.value = false
    router.push({ path: '/' })
  }, 500)
}

function reportLoginError(error: unknown) {
  const response = error as Parameters<typeof apiRespErrMsg>[0] | undefined
  if (!response || !apiRespErrMsg(response))
    ms.error(response?.msg || t('common.unknownError'))
}

function refreshCaptcha() {
  if (captchaEnabled.value)
    captchaRef.value?.refresh()
}

async function changeDefaultPassword() {
  if (loading.value)
    return
  loading.value = true
  try {
    const response = await updatePassword(form.value.password, newPassword.value, passwordChangeToken.value)
    if (response.code !== 0) {
      if (response.code === 1001)
        passwordChangeToken.value = ''
      return
    }
    form.value.password = ''
    form.value.totpCode = ''
    form.value.vcode = ''
    newPassword.value = ''
    passwordChangeToken.value = ''
    totpRequired.value = false
    refreshCaptcha()
    ms.success(t('login.defaultPasswordChanged'))
  }
  catch (error) {
    reportLoginError(error)
  }
  finally {
    loading.value = false
  }
}

const loginPost = async () => {
  if (loading.value || passkeyLoading.value)
    return
  loading.value = true
  try {
    const payload: Login.LoginReqest = { ...form.value }
    // 未进入动态码阶段时不提交该字段
    if (!totpRequired.value)
      delete payload.totpCode

    const res = await login<Login.LoginResponse>(payload)
    if (res.code === 0) {
      handleLoginSuccess(res.data)
      return
    }
    throw res
  }
  catch (error) {
    const code = (error as { code?: number } | null)?.code

    // 密码正确但需要动态验证码（业务码 1101）
    if (code === 1101) {
      totpRequired.value = true
      form.value.totpCode = ''
      // 重试登录会重新校验图形验证码，需刷新
      form.value.vcode = ''
      refreshCaptcha()
      nextTick(() => inputTotpRef.value?.focus())
      ms.info(t('login.totpRequiredTip'))
      loading.value = false
      return
    }

    // 动态码错误或已被使用（业务码 1102）
    if (code === 1102) {
      form.value.totpCode = ''
      if (inputTotpRef.value)
        inputTotpRef.value.value = ''
      form.value.vcode = ''
      refreshCaptcha()
      nextTick(() => {
        if (totpRequired.value)
          inputTotpRef.value?.focus()
        else
          inputpasswordRef.value?.focus()
      })
      ms.error(t('apiErrorCode.1102'))
      loading.value = false
      return
    }

    form.value.vcode = ''
    refreshCaptcha()
    reportLoginError(error)
    loading.value = false
  }
}

// Passkey 登录（discoverable，无需用户名）
const passkeyLogin = async () => {
  if (passkeyLoading.value || loading.value)
    return
  passkeyLoading.value = true
  try {
    const { data } = await passkeyLoginBegin<Security.PublicKeyFlowResponse>()
    const options = toRequestOptions(data.publicKey)
    const credential = await navigator.credentials.get({ publicKey: options }) as PublicKeyCredential | null
    if (!credential) {
      const emptyError = new Error('NotAllowedError')
      emptyError.name = 'NotAllowedError'
      throw emptyError
    }
    const { data: loginData } = await passkeyLoginFinish<Login.LoginResponse>(
      data.flowId,
      serializeAssertionCredential(credential),
    )
    handleLoginSuccess(loginData)
  }
  catch (error) {
    passkeyLoading.value = false
    if ((error as { code?: number } | null)?.code !== undefined) {
      reportLoginError(error)
      return
    }
    const failure = classifyWebAuthnError(error)
    if (failure === 'cancelled')
      ms.warning(t('login.passkeyCancelled'))
    else if (failure === 'unsupported')
      ms.error(t('login.passkeyUnsupported'))
    else
      ms.error(t('common.unknownError'))
  }
}

function handleSubmit(event: Event) {
  event.preventDefault()
  const target = event.currentTarget
  if (!(target instanceof HTMLFormElement))
    return
  const fields = new FormData(target)
  form.value.password = String(fields.get('password') ?? '')
  if (mustChangePassword.value) {
    newPassword.value = String(fields.get('new-password') ?? '')
    void changeDefaultPassword()
    return
  }
  form.value.username = String(fields.get('username') ?? '')
  form.value.totpCode = String(fields.get('otp') ?? '')
  void loginPost()
}

function handleChangeLanuage(value: Language) {
  languageValue.value = value
  appStore.setLanguage(value)
  history.go(0)
}

async function getConfig() {
  try {
    const { data } = await get<{ captchaEnabled: boolean; passkeyAvailable: boolean }>({ url: '/login/options' })
    captchaEnabled.value = data.captchaEnabled
    passkeyConfigured.value = data.passkeyAvailable
    loginOptionsLoaded.value = true
  }
  catch (error) {
    reportLoginError(error)
  }
}

function handleTheme(value: boolean) {
  appStore.setTheme(value ? 'dark' : 'light')
}

onMounted(() => {
  getConfig()
  if (defaultUsername.value !== '') {
    form.value.username = defaultUsername.value
    inputpasswordRef.value?.focus()
  }

  // 如果已经登录并且不是在添加账号登录页面，跳转到首页
  if (route.query.type !== 'addLogin' && authStore.token) {
    router.push({ path: '/' })
  }
})
</script>

<template>
  <div>
    <div class="background" />
    <div class="login-container">
      <NCard class="login-card" style="border-radius: 20px;">
        <div class="mb-5 flex items-center justify-end">
          <div class="w-full">
            <NSwitch v-model:value="isDark" @update:value="handleTheme">
              <template #checked-icon>
                <SvgIcon icon="mingcute:moon-fog-line" />
              </template>
              <SvgIcon icon="tabler-sun-filled" />
              <template #unchecked-icon>
                <SvgIcon icon="mingcute:sun-fog-line" />
              </template>
            </NSwitch>
          </div>
          <div class="mr-1">
            <SvgIcon
              icon="flowbite-language-outline"
              style="width: 22px;height: 22px;"
              class="language-text-style"
            />
          </div>
          <div class="min-w-[100px]">
            <NSelect
              v-model:value="languageValue" size="small" :options="languageOptions"
              @update-value="handleChangeLanuage"
            />
          </div>
        </div>

        <div class="login-title">
          <!-- <NGradientText :size="30" type="success" class="!font-bold"> -->
          Sun-Panel
          <!-- </NGradientText> -->
        </div>
        <NForm :model="form" label-width="100px" :on-submit="handleSubmit">
          <template v-if="mustChangePassword">
            <p>{{ t('login.changeDefaultPassword') }}</p>
            <NFormItem>
              <NInput v-model:value="form.password" type="password" :input-props="{ name: 'password', autocomplete: 'current-password' }" :placeholder="t('settingUserInfo.oldPassword')" />
            </NFormItem>
            <NFormItem>
              <NInput v-model:value="newPassword" type="password" :input-props="{ name: 'new-password', autocomplete: 'new-password' }" :placeholder="t('login.newPassword')" />
            </NFormItem>
            <NButton :loading="loading" class="login-button" attr-type="submit">
              {{ t('login.savePassword') }}
            </NButton>
          </template>
          <template v-else>
          <NFormItem>
            <NInput v-model:value="form.username" :input-props="{ id: 'login-username', name: 'username', autocomplete: 'username' }" :placeholder="$t('login.usernamePlaceholder')">
              <template #prefix>
                <SvgIcon icon="ph:user-bold" />
              </template>
            </NInput>
          </NFormItem>

          <NFormItem>
            <NInput ref="inputpasswordRef" v-model:value="form.password" type="password" :input-props="{ id: 'login-password', name: 'password', autocomplete: 'current-password' }" :placeholder="$t('login.passwordPlaceholder')">
              <template #prefix>
                <SvgIcon icon="mdi:password-outline" />
              </template>
            </NInput>
          </NFormItem>

          <NFormItem v-if="totpRequired">
            <input
              id="login-otp" ref="inputTotpRef" name="otp" type="text" class="login-otp"
              maxlength="6" autocomplete="one-time-code" inputmode="numeric"
              :aria-label="t('login.totpCodePlaceholder')" :placeholder="t('login.totpCodePlaceholder')"
            >
          </NFormItem>

          <NFormItem v-if="captchaEnabled">
            <div class="w-[130px] h-[40px] mr-[20px] rounded border flex cursor-pointer">
              <Captcha ref="captchaRef" src="/api/captcha/getImage" />
            </div>
            <NInput v-model:value="form.vcode" size="large" type="text" clearable :placeholder="t('login.captchaCodePlaceholder')" />
          </NFormItem>

          <NFormItem style="margin-top: 10px">
            <NButton :loading="loading" class="login-button" attr-type="submit">
              {{ $t('login.loginButton') }}
            </NButton>
          </NFormItem>
          </template>
        </NForm>

        <template v-if="webAuthnSupport !== 'unsupported'">
          <NDivider v-if="!mustChangePassword" style="margin: 4px 0 12px;">
            {{ t('login.dividerOr') }}
          </NDivider>
          <NButton
            v-if="!mustChangePassword" class="login-button" :loading="passkeyLoading" :disabled="webAuthnSupport !== 'ok' || !passkeyConfigured"
            @click="passkeyLogin"
          >
            <template #icon>
              <SvgIcon icon="mdi:fingerprint" />
            </template>
            {{ t('login.passkeyLogin') }}
          </NButton>
          <div v-if="webAuthnSupport === 'insecure'" class="passkey-https-tip">
            {{ t('login.passkeyNeedHttps') }}
          </div>
          <div v-else-if="loginOptionsLoaded && !passkeyConfigured && !mustChangePassword" class="passkey-https-tip">
            {{ t('apps.userInfo.security.passkeyUnconfigured') }}
          </div>
        </template>

        <div class="copyright flex justify-center text-current">
          Powered By <a
            href="https://github.com/Seameee/sun-panel" target="_blank" rel="noopener noreferrer"
            class="ml-[5px]"
          >Sun-Panel</a>
        </div>
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.background {
  position: absolute;
  width: 100%;
  height: 100%;
  background-size: cover;
  background-position: center;
  background-color: #f2f6ff;
}

.dark .background {
  filter: brightness(80%);
  background-color: #18181b;
}

.login-container {
  padding: 20px;
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
}

.login-title {
  font-size: 30px;
  font-weight: 900;
  text-align: center;
  margin: 20px;
}

@media (min-width: 600px) {
  .login-card {
    width: auto;
    margin: 0px 10px;
  }
}

.login-button {
  width: 100%;
  color: var(--n-text-color);
  background-color: white;
  font-weight: 900 !important;
}

.dark .login-button {
  background-color: transparent;
}

.login-card {
  margin: 20px;
  min-width: 400px;
  background-color: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(10px);
}

.dark .login-card {
  filter: brightness(90%);
  background-color: rgba(72, 72, 72, 0.4);
  backdrop-filter: blur(10px);
}

.language-text-style{
 color: var(--n-text-color);
}

.login-otp {
  width: 100%;
  height: 34px;
  border: 1px solid rgba(128, 128, 128, 0.35);
  border-radius: 3px;
  padding: 0 12px;
  background: rgba(255, 255, 255, 0.75);
  color: inherit;
}

.dark .login-otp {
  background: rgba(255, 255, 255, 0.07);
}

.login-otp:focus-visible {
  outline: 2px solid var(--n-primary-color, #18a058);
  outline-offset: 1px;
}

.passkey-https-tip {
  margin-top: 8px;
  font-size: 12px;
  text-align: center;
  color: var(--n-text-color);
  opacity: 0.65;
}
</style>
