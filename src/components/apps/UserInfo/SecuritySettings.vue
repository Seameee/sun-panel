<script setup lang="ts">
import { NButton, NCard, NDivider, NFlex, NInput, NSpin, NTag, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { encode } from 'uqr'
import { RoundCardModal } from '@/components/common'
import {
  getSecurityStatus,
  securityPasskeyBegin,
  securityPasskeyDelete as securityPasskeyDeleteApi,
  securityPasskeyFinish,
  securityTotpBegin,
  securityTotpDisable as securityTotpDisableApi,
  securityTotpFinish,
} from '@/api/security'
import { copyToClipboard, timeFormat } from '@/utils/cmn'
import { apiRespErrMsg } from '@/utils/request/apiMessage'
import {
  classifyWebAuthnError,
  isWebAuthnError,
  getWebAuthnSupport,
  serializeCreationCredential,
  toCreationOptions,
} from '@/utils/webauthn'
import { t } from '@/locales'

const ms = useMessage()

const webAuthnSupport = getWebAuthnSupport()
const statusLoading = ref(true)
const status = ref<Security.StatusResponse>({
  totpEnabled: false,
  passkeys: [],
  passkeyAvailable: false,
})

const totpEnable = ref({
  show: false,
  step: 'password' as 'password' | 'verify',
  loading: false,
  flowId: '',
  secret: '',
  otpauthUrl: '',
})

const totpDisable = ref({
  show: false,
  loading: false,
})

const passkeyCreate = ref({
  show: false,
  step: 'password' as 'password' | 'waiting',
  loading: false,
  name: '',
})

const passkeyDelete = ref({
  show: false,
  loading: false,
  id: '' as string | number,
  name: '',
})

// 本地生成二维码，密钥不出浏览器
const qr = computed(() => {
  const url = totpEnable.value.otpauthUrl
  if (!url)
    return null
  try {
    const { data } = encode(url, { ecc: 'M', border: 0 })
    const rects: { x: number; y: number }[] = []
    data.forEach((row, y) => {
      row.forEach((filled, x) => {
        if (filled)
          rects.push({ x, y })
      })
    })
    return { size: data.length, rects }
  }
  catch {
    // 链接过长等情况下退回手动输入密钥
    return null
  }
})

function formValues(event: SubmitEvent) {
  const fields = new FormData(event.currentTarget as HTMLFormElement)
  return {
    password: String(fields.get('password') ?? ''),
    code: String(fields.get('otp') ?? '').replace(/[\s-]/g, ''),
  }
}

function validCode(code: string) {
  return /^\d{6}$/.test(code)
}

function formatTime(value?: string) {
  return value && Number.isFinite(Date.parse(value)) ? timeFormat(value) : ''
}

function resetTotpEnable() {
  totpEnable.value = {
    show: false,
    step: 'password',
    loading: false,
        flowId: '',
    secret: '',
    otpauthUrl: '',
  }
}

function resetTotpDisable() {
  totpDisable.value = { show: false, loading: false }
}

function resetPasskeyCreate() {
  passkeyCreate.value = { show: false, step: 'password', loading: false, name: '' }
}

function resetPasskeyDelete() {
  passkeyDelete.value = { show: false, loading: false, id: '', name: '' }
}

function openTotpEnable() {
  resetTotpEnable()
  totpEnable.value.show = true
}

function openTotpDisable() {
  resetTotpDisable()
  totpDisable.value.show = true
}

function openPasskeyCreate() {
  resetPasskeyCreate()
  passkeyCreate.value.show = true
}

function openPasskeyDelete(item: Security.PasskeyInfo) {
  resetPasskeyDelete()
  passkeyDelete.value.id = item.id
  passkeyDelete.value.name = item.name
  passkeyDelete.value.show = true
}

function handleTotpEnableShow(show: boolean) {
  if (!show)
    resetTotpEnable()
}

function handleTotpDisableShow(show: boolean) {
  if (!show)
    resetTotpDisable()
}

function handlePasskeyCreateShow(show: boolean) {
  if (!show)
    resetPasskeyCreate()
}

function handlePasskeyDeleteShow(show: boolean) {
  if (!show)
    resetPasskeyDelete()
}

async function loadStatus() {
  statusLoading.value = true
  try {
    const { data } = await getSecurityStatus<Security.StatusResponse>()
    status.value = {
      totpEnabled: !!data?.totpEnabled,
      passkeys: data?.passkeys || [],
      passkeyAvailable: !!data?.passkeyAvailable,
    }
  }
  catch (error) {
    apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
  }
  finally {
    statusLoading.value = false
  }
}

function buildCredentialError(name: string) {
  const error = new Error(name)
  error.name = name
  return error
}

function handleWebAuthnFailure(error: unknown) {
  if (!isWebAuthnError(error)) {
    if ((error as { code?: number } | null)?.code !== undefined)
      apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
    else
      ms.error(t('common.unknownError'))
    return
  }

  const failure = classifyWebAuthnError(error)
  if (failure === 'cancelled')
    ms.warning(t('apps.userInfo.security.passkeyCancelled'))
  else if (failure === 'exists')
    ms.warning(t('apps.userInfo.security.passkeyExists'))
  else
    ms.error(t('apps.userInfo.security.passkeyUnsupported'))
}

async function copySecret() {
  const ok = await copyToClipboard(totpEnable.value.secret)
  if (ok)
    ms.success(t('apps.userInfo.security.copySuccess'))
  else
    ms.error(t('apps.userInfo.security.copyFail'))
}

async function submitTotpPassword(event: SubmitEvent) {
  if (totpEnable.value.loading)
    return
  const { password } = formValues(event)
  if (!password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  totpEnable.value.loading = true
  try {
    const { data } = await securityTotpBegin<Security.TotpBeginResponse>(password)
    totpEnable.value.flowId = data.flowId
    totpEnable.value.secret = data.secret
    totpEnable.value.otpauthUrl = data.otpauthUrl
    totpEnable.value.step = 'verify'
  }
  catch (error) {
    apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
  }
  finally {
    totpEnable.value.loading = false
  }
}

async function submitTotpVerify(event: SubmitEvent) {
  if (totpEnable.value.loading)
    return
  const { code } = formValues(event)
  if (!validCode(code)) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }
  totpEnable.value.loading = true
  try {
    await securityTotpFinish(totpEnable.value.flowId, code)
    ms.success(t('apps.userInfo.security.totpEnableSuccess'))
    resetTotpEnable()
    await loadStatus()
  }
  catch (error) {
    apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
  }
  finally {
    totpEnable.value.loading = false
  }
}

async function submitTotpDisable(event: SubmitEvent) {
  if (totpDisable.value.loading)
    return
  const { password, code } = formValues(event)
  if (!password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  if (!validCode(code)) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }
  totpDisable.value.loading = true
  try {
    await securityTotpDisableApi(password, code)
    ms.success(t('apps.userInfo.security.totpDisableSuccess'))
    resetTotpDisable()
    await loadStatus()
  }
  catch (error) {
    apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
  }
  finally {
    totpDisable.value.loading = false
  }
}

async function submitPasskeyCreate(event: SubmitEvent) {
  if (passkeyCreate.value.loading || passkeyCreate.value.step !== 'password')
    return
  const { password, code: totpCode } = formValues(event)
  if (!password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  const name = passkeyCreate.value.name.trim() || t('apps.userInfo.security.passkeyDefaultName')
  if (status.value.totpEnabled && !validCode(totpCode)) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }

  passkeyCreate.value.loading = true
  try {
    const { data } = await securityPasskeyBegin<Security.PublicKeyFlowResponse>(
      password,
      name,
      status.value.totpEnabled ? totpCode : undefined,
    )

    const options = toCreationOptions(data.publicKey)
    passkeyCreate.value.step = 'waiting'
    const credential = await navigator.credentials.create({ publicKey: options }) as PublicKeyCredential | null
    if (!credential)
      throw buildCredentialError('NotAllowedError')

    await securityPasskeyFinish(data.flowId, serializeCreationCredential(credential))
    ms.success(t('apps.userInfo.security.passkeyAddSuccess'))
    resetPasskeyCreate()
    await loadStatus()
  }
  catch (error) {
    handleWebAuthnFailure(error)
    passkeyCreate.value.step = 'password'
  }
  finally {
    passkeyCreate.value.loading = false
  }
}

async function submitPasskeyDelete(event: SubmitEvent) {
  if (passkeyDelete.value.loading)
    return
  const { password, code: totpCode } = formValues(event)
  if (!password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  if (status.value.totpEnabled && !validCode(totpCode)) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }

  passkeyDelete.value.loading = true
  try {
    await securityPasskeyDeleteApi(
      passkeyDelete.value.id,
      password,
      status.value.totpEnabled ? totpCode : undefined,
    )
    ms.success(t('apps.userInfo.security.passkeyDeleteSuccess'))
    resetPasskeyDelete()
    await loadStatus()
  }
  catch (error) {
    apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
  }
  finally {
    passkeyDelete.value.loading = false
  }
}

onMounted(() => {
  loadStatus()
})
</script>

<template>
  <NCard style="border-radius:10px" size="small">
    <template #header>
      <NFlex align="center" :size="6" class="text-slate-500 font-bold">
        {{ t('apps.userInfo.security.title') }}
        <NSpin v-if="statusLoading" :size="14" />
      </NFlex>
    </template>

    <!-- 动态口令 TOTP -->
    <div>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span class="font-bold">{{ t('apps.userInfo.security.totpTitle') }}</span>
          <NTag size="small" :type="status.totpEnabled ? 'success' : 'default'">
            {{ status.totpEnabled ? t('apps.userInfo.security.enabled') : t('apps.userInfo.security.disabled') }}
          </NTag>
        </div>
        <NButton size="small" text :type="status.totpEnabled ? 'error' : 'primary'" @click="status.totpEnabled ? openTotpDisable() : openTotpEnable()">
          {{ status.totpEnabled ? t('apps.userInfo.security.totpDisable') : t('apps.userInfo.security.totpEnable') }}
        </NButton>
      </div>
      <div class="text-slate-500 text-xs mt-1">
        {{ t('apps.userInfo.security.totpDesc') }}
      </div>
    </div>

    <NDivider style="margin: 12px 0;" dashed />

    <!-- Passkey -->
    <div>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span class="font-bold">{{ t('apps.userInfo.security.passkeyTitle') }}</span>
          <NTag size="small" :type="status.passkeys.length > 0 ? 'success' : 'default'">
            {{ t('apps.userInfo.security.passkeyCount', { count: status.passkeys.length }) }}
          </NTag>
        </div>
        <NButton size="small" text type="primary" :disabled="statusLoading || !status.passkeyAvailable || webAuthnSupport !== 'ok'" @click="openPasskeyCreate">
          {{ t('apps.userInfo.security.passkeyAdd') }}
        </NButton>
      </div>
      <div v-if="!statusLoading" class="text-slate-500 text-xs mt-1 break-words">
        {{ webAuthnSupport === 'insecure' ? t('login.passkeyNeedHttps')
          : webAuthnSupport === 'unsupported' ? t('login.passkeyUnsupported')
          : status.passkeyAvailable ? t('apps.userInfo.security.passkeyDesc')
          : t('apps.userInfo.security.passkeyUnconfigured') }}
      </div>

      <div v-if="status.passkeys.length > 0" class="mt-2">
        <div v-for="item in status.passkeys" :key="item.id" class="flex items-center justify-between gap-2 py-1">
          <div class="min-w-0">
            <div class="text-sm truncate">
              {{ item.name || t('apps.userInfo.security.passkeyDefaultName') }}
            </div>
            <div class="text-slate-500 text-xs">
              {{ formatTime(item.createdAt) }}
            </div>
          </div>
          <NButton size="tiny" text type="error" @click="openPasskeyDelete(item)">
            {{ t('common.delete') }}
          </NButton>
        </div>
      </div>
      <div v-else class="text-slate-500 text-xs mt-2">
        {{ t('apps.userInfo.security.passkeyEmpty') }}
      </div>
    </div>

    <!-- 启用 TOTP -->
    <RoundCardModal
      v-model:show="totpEnable.show" size="small" preset="card" style="width: 420px"
      :title="t('apps.userInfo.security.totpEnableTitle')" @update:show="handleTotpEnableShow"
    >
      <form v-if="totpEnable.step === 'password'" id="totp-enable-password" @submit.prevent="submitTotpPassword">
        <div class="text-slate-500 text-xs mb-2">
          {{ t('apps.userInfo.security.totpPasswordTip') }}
        </div>
        <input
          class="security-input" name="password" type="password" autocomplete="current-password"
          :placeholder="t('common.password')" :aria-label="t('common.password')"
        >
      </form>

      <form v-else id="totp-enable-verify" @submit.prevent="submitTotpVerify">
        <div class="text-slate-500 text-xs">
          {{ t('apps.userInfo.security.totpScanTip') }}
        </div>

        <div v-if="qr" class="flex justify-center my-3">
          <div class="p-2 rounded" style="background-color: #ffffff;">
            <svg
              :viewBox="`0 0 ${qr.size} ${qr.size}`" width="152" height="152"
              shape-rendering="crispEdges" role="img" :aria-label="t('apps.userInfo.security.totpQrLabel')"
            >
              <rect :width="qr.size" :height="qr.size" fill="#ffffff" />
              <rect v-for="(m, index) in qr.rects" :key="index" :x="m.x" :y="m.y" width="1" height="1" fill="#000000" />
            </svg>
          </div>
        </div>

        <div class="mt-2">
          <div class="text-slate-500 text-xs">
            {{ t('apps.userInfo.security.totpSecretLabel') }}
          </div>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <code class="text-sm break-all">{{ totpEnable.secret }}</code>
            <NButton size="tiny" text type="primary" @click="copySecret">
              {{ t('apps.userInfo.security.copy') }}
            </NButton>
          </div>
        </div>

        <div class="mt-3">
          <div class="text-slate-500 text-xs mb-1">
            {{ t('apps.userInfo.security.codeLabel') }}
          </div>
          <input
            class="security-input" name="otp" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="6"
            :placeholder="t('apps.userInfo.security.codePlaceholder')" :aria-label="t('apps.userInfo.security.codeLabel')"
          >
        </div>

        <div class="text-slate-500 text-xs mt-2">
          {{ t('apps.userInfo.security.totpReaddTip') }}
        </div>
      </form>

      <template #footer>
        <div class="float-right">
          <NButton
            size="small" type="primary" :loading="totpEnable.loading"
            attr-type="submit" :form="totpEnable.step === 'password' ? 'totp-enable-password' : 'totp-enable-verify'"
          >
            {{ totpEnable.step === 'password' ? t('common.continue') : t('common.save') }}
          </NButton>
        </div>
      </template>
    </RoundCardModal>

    <!-- 关闭 TOTP -->
    <RoundCardModal
      v-model:show="totpDisable.show" size="small" preset="card" style="width: 420px"
      :title="t('apps.userInfo.security.totpDisableTitle')" @update:show="handleTotpDisableShow"
    >
      <form id="totp-disable" @submit.prevent="submitTotpDisable">
        <div class="text-slate-500 text-xs mb-2">
          {{ t('apps.userInfo.security.totpDisableTip') }}
        </div>
        <input
          class="security-input" name="password" type="password" autocomplete="current-password"
          :placeholder="t('common.password')" :aria-label="t('common.password')"
        >
        <div class="mt-2">
          <input
            class="security-input" name="otp" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="6"
            :placeholder="t('apps.userInfo.security.codePlaceholder')" :aria-label="t('apps.userInfo.security.codeLabel')"
          >
        </div>
      </form>
      <template #footer>
        <div class="float-right">
          <NButton size="small" type="error" :loading="totpDisable.loading" attr-type="submit" form="totp-disable">
            {{ t('apps.userInfo.security.totpDisable') }}
          </NButton>
        </div>
      </template>
    </RoundCardModal>

    <!-- 添加 Passkey -->
    <RoundCardModal
      v-model:show="passkeyCreate.show" size="small" preset="card" style="width: 420px"
      :title="t('apps.userInfo.security.passkeyAdd')" @update:show="handlePasskeyCreateShow"
    >
      <form v-if="passkeyCreate.step === 'password'" id="passkey-create" @submit.prevent="submitPasskeyCreate">
        <div class="text-slate-500 text-xs mb-2">
          {{ t('apps.userInfo.security.passkeyAddTip') }}
        </div>
        <div class="text-slate-500 text-xs mb-1">
          {{ t('apps.userInfo.security.passkeyName') }}
        </div>
        <NInput
          v-model:value="passkeyCreate.name" :maxlength="30"
          :placeholder="t('apps.userInfo.security.passkeyNamePlaceholder')"
        />
        <div class="text-slate-500 text-xs mb-1 mt-2">
          {{ t('common.password') }}
        </div>
        <input
          class="security-input" name="password" type="password" autocomplete="current-password"
          :placeholder="t('common.password')" :aria-label="t('common.password')"
        >
        <div v-if="status.totpEnabled" class="mt-2">
          <div class="text-slate-500 text-xs mb-1">
            {{ t('apps.userInfo.security.codeLabel') }}
          </div>
          <input
            class="security-input" name="otp" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="6"
            :placeholder="t('apps.userInfo.security.codePlaceholder')" :aria-label="t('apps.userInfo.security.codeLabel')"
          >
        </div>
      </form>
      <div v-else class="flex flex-col items-center py-4">
        <NSpin size="medium" />
        <div class="text-slate-500 text-xs mt-2">
          {{ t('apps.userInfo.security.passkeyWaiting') }}
        </div>
      </div>
      <template #footer>
        <div class="float-right">
          <NButton
            size="small" type="primary" :loading="passkeyCreate.loading"
            :disabled="passkeyCreate.step === 'waiting'" attr-type="submit" form="passkey-create"
          >
            {{ t('apps.userInfo.security.passkeyAdd') }}
          </NButton>
        </div>
      </template>
    </RoundCardModal>

    <!-- 删除 Passkey -->
    <RoundCardModal
      v-model:show="passkeyDelete.show" size="small" preset="card" style="width: 420px"
      :title="t('apps.userInfo.security.passkeyDelete')" @update:show="handlePasskeyDeleteShow"
    >
      <form id="passkey-delete" @submit.prevent="submitPasskeyDelete">
        <div class="text-slate-500 text-xs mb-2">
          {{ t('apps.userInfo.security.passkeyDeleteTip', { name: passkeyDelete.name || t('apps.userInfo.security.passkeyDefaultName') }) }}
        </div>
        <input
          class="security-input" name="password" type="password" autocomplete="current-password"
          :placeholder="t('common.password')" :aria-label="t('common.password')"
        >
        <div v-if="status.totpEnabled" class="mt-2">
          <div class="text-slate-500 text-xs mb-1">
            {{ t('apps.userInfo.security.codeLabel') }}
          </div>
          <input
            class="security-input" name="otp" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="6"
            :placeholder="t('apps.userInfo.security.codePlaceholder')" :aria-label="t('apps.userInfo.security.codeLabel')"
          >
        </div>
      </form>
      <template #footer>
        <div class="float-right">
          <NButton size="small" type="error" :loading="passkeyDelete.loading" attr-type="submit" form="passkey-delete">
            {{ t('common.delete') }}
          </NButton>
        </div>
      </template>
    </RoundCardModal>
  </NCard>
</template>

<style scoped>
.security-input {
  display: block;
  width: 100%;
  height: 34px;
  border: 1px solid var(--n-border-color, #80808059);
  border-radius: 3px;
  padding: 0 12px;
  background: transparent;
  color: inherit;
}

.security-input:focus-visible {
  outline: 2px solid var(--n-primary-color, #18a058);
  outline-offset: 1px;
}
</style>
