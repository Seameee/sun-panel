<script setup lang="ts">
import { NButton, NCard, NDivider, NForm, NFormItem, NInput, NSelect, NSwitch, useMessage, useOsTheme } from 'naive-ui'
import type { InputInst } from 'naive-ui'
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Captcha, SvgIcon } from '@/components/common'
import { login } from '@/api'
import { passkeyLoginBegin, passkeyLoginFinish } from '@/api/security'
import { useAccountManagerStore, useAppStore, useAuthStore, usePanelState } from '@/store'
import { router } from '@/router'
import { t } from '@/locales'
import { languageOptions } from '@/utils/defaultData'
import type { Language } from '@/store/modules/app/helper'
import { getSiteStting } from '@/api/openness'
import { apiRespErrMsg, setTitle } from '@/utils/cmn'
import { classifyWebAuthnError, getWebAuthnSupport, serializeAssertionCredential, toRequestOptions } from '@/utils/webauthn'
import { defaultSetting } from '@/store/modules/app/helper'

const route = useRoute()
// const userStore = useUserStore()
const authStore = useAuthStore()
const appStore = useAppStore()
const panelState = usePanelState()
const accountManagerStore = useAccountManagerStore()
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
const siteSetting = ref<GlobalSetting.SiteSetting>(defaultSetting().siteSetting)
const captchaRef = ref()

const form = ref<Login.LoginReqest>({
  username: defaultUsername.value,
  password: '',
})
const totpRequired = ref(false)
const inputTotpRef = ref<InputInst | null>(null)
const passkeyLoading = ref(false)
const webAuthnSupport = getWebAuthnSupport()

// 登录成功后的统一处理（密码登录与 Passkey 登录共用）
function handleLoginSuccess(data: Login.LoginResponse) {
  authStore.setToken(data.token)
  authStore.setUserInfo(data)
  panelState.reset() // 重置面板配置

  // 添加到账号管理器
  accountManagerStore.addAccount({
    username: data.username || form.value.username,
    name: data.name || '',
    id: data.id || 0,
    cToken: data.token,
    loginStatus: true,
  })

  setTimeout(() => {
    ms.success(`Hi ${data.name},${t('login.welcomeMessage')}`)
    loading.value = false
    passkeyLoading.value = false
    router.push({ path: '/' })
  }, 500)
}

function refreshCaptcha() {
  if (siteSetting.value.isEnableLoginCaptcha)
    captchaRef.value?.refresh()
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
    loading.value = false
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

    refreshCaptcha()
    apiRespErrMsg(error)
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
      apiRespErrMsg(error)
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

function handleEnterDown(e: any) {
  // 修复Mac safari浏览器中文输入法回车触发
  if (e.keyCode === 229)
    return
  if (e.key === 'Enter' && !e.isComposing) {
    // 点击登录按钮触发
    loginPost()
  }
}

function handleChangeLanuage(value: Language) {
  languageValue.value = value
  appStore.setLanguage(value)
  history.go(0)
}

async function getConfig() {
  try {
    const { data } = await getSiteStting<GlobalSetting.SiteSetting>()
    const dcfg = defaultSetting().siteSetting
    if (data.title === '')
      data.title = dcfg.title
    if (data.loginBackgroundUrl === '')
      data.loginBackgroundUrl = dcfg.loginBackgroundUrl

    appStore.setSiteSetting(data)
    siteSetting.value = data
    setTitle(data.title)
  }
  catch (error) {
    apiRespErrMsg(error)
  }
}

function handleTheme(value: boolean) {
  appStore.setTheme(value ? 'dark' : 'light')
}

onMounted(() => {
  setTitle(appStore.siteSetting.title)
  getConfig()
  if (defaultUsername.value !== '') {
    form.value.username = defaultUsername.value
    inputpasswordRef.value?.focus()
  }

  // 如果已经登录并且不是在添加账号登录页面，跳转到首页
  if (route.query.type !== 'addLogin' && (authStore.token !== null && authStore.token !== '')) {
    router.push({ path: '/' })
  }
})
</script>

<template>
  <div>
    <div class="background" :style="{ backgroundImage: `url('${appStore.siteSetting.loginBackgroundUrl}')` }" />
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
          {{ appStore.siteSetting.title }}
          <!-- </NGradientText> -->
        </div>
        <NForm :model="form" label-width="100px" @keydown.enter="handleEnterDown">
          <NFormItem>
            <NInput v-model:value="form.username" :placeholder="$t('login.usernamePlaceholder')">
              <template #prefix>
                <SvgIcon icon="ph:user-bold" />
              </template>
            </NInput>
          </NFormItem>

          <NFormItem>
            <NInput ref="inputpasswordRef" v-model:value="form.password" type="password" :placeholder="$t('login.passwordPlaceholder')">
              <template #prefix>
                <SvgIcon icon="mdi:password-outline" />
              </template>
            </NInput>
          </NFormItem>

          <NFormItem v-if="totpRequired">
            <NInput
              ref="inputTotpRef" v-model:value="form.totpCode" type="text" :maxlength="6"
              inputmode="numeric" :placeholder="t('login.totpCodePlaceholder')"
              :aria-label="t('login.totpCodePlaceholder')"
            >
              <template #prefix>
                <SvgIcon icon="mdi:shield-key-outline" />
              </template>
            </NInput>
          </NFormItem>

          <NFormItem v-if="appStore.siteSetting.isEnableLoginCaptcha">
            <div class="w-[130px] h-[40px] mr-[20px] rounded border flex cursor-pointer">
              <Captcha ref="captchaRef" src="/api/captcha/getImage" />
            </div>
            <NInput v-model:value="form.vcode" size="large" type="text" clearable :placeholder="t('login.captchaCodePlaceholder')" />
          </NFormItem>

          <NFormItem style="margin-top: 10px">
            <NButton :loading="loading" class="login-button" @click="loginPost">
              {{ $t('login.loginButton') }}
            </NButton>
          </NFormItem>
        </NForm>

        <template v-if="webAuthnSupport !== 'unsupported'">
          <NDivider style="margin: 4px 0 12px;">
            {{ t('login.dividerOr') }}
          </NDivider>
          <NButton
            class="login-button" :loading="passkeyLoading" :disabled="webAuthnSupport !== 'ok'"
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
        </template>

        <div class="copyright flex justify-center text-current">
          Powered By <a
            href="https://github.com/hslr-s/sun-panel" target="_blank"
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

.passkey-https-tip {
  margin-top: 8px;
  font-size: 12px;
  text-align: center;
  color: var(--n-text-color);
  opacity: 0.65;
}
</style>
