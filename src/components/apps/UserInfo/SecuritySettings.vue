<script setup lang="ts">
import { NButton, NCard, NDivider, NFlex, NInput, NSpin, NTag, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { encode } from 'uqr'
import { RoundCardModal, SvgIcon } from '@/components/common'
import {
  getSecurityStatus,
  securityPasskeyBegin,
  securityPasskeyDelete as securityPasskeyDeleteApi,
  securityPasskeyFinish,
  securityTotpBegin,
  securityTotpDisable as securityTotpDisableApi,
  securityTotpFinish,
} from '@/api/security'
import { buildTimeString, copyToClipboard } from '@/utils/cmn'
import { apiRespErrMsg } from '@/utils/request/apiMessage'
import {
  classifyWebAuthnError,
  isWebAuthnError,
  serializeCreationCredential,
  toCreationOptions,
} from '@/utils/webauthn'
import { t } from '@/locales'

const ms = useMessage()

const statusLoading = ref(false)
const status = ref<Security.StatusResponse>({
  totpEnabled: false,
  passkeys: [],
  passkeyAvailable: false,
})

const totpEnable = ref({
  show: false,
  step: 'password' as 'password' | 'verify',
  loading: false,
  password: '',
  code: '',
  flowId: '',
  secret: '',
  otpauthUrl: '',
})

const totpDisable = ref({
  show: false,
  loading: false,
  password: '',
  code: '',
})

const passkeyCreate = ref({
  show: false,
  step: 'password' as 'password' | 'waiting',
  loading: false,
  password: '',
  name: '',
  totpCode: '',
})

const passkeyDelete = ref({
  show: false,
  loading: false,
  id: '' as string | number,
  name: '',
  password: '',
  totpCode: '',
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

const onlyDigits = (value: string) => /^\d*$/.test(value)

function normalizeCode(value: string) {
  return (value || '').replace(/[\s-]/g, '')
}

function formatTime(value?: string) {
  if (!value)
    return ''
  return buildTimeString(value)
}

function resetTotpEnable() {
  totpEnable.value = {
    show: false,
    step: 'password',
    loading: false,
    password: '',
    code: '',
    flowId: '',
    secret: '',
    otpauthUrl: '',
  }
}

function resetTotpDisable() {
  totpDisable.value = { show: false, loading: false, password: '', code: '' }
}

function resetPasskeyCreate() {
  passkeyCreate.value = { show: false, step: 'password', loading: false, password: '', name: '', totpCode: '' }
}

function resetPasskeyDelete() {
  passkeyDelete.value = { show: false, loading: false, id: '', name: '', password: '', totpCode: '' }
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

async function submitTotpPassword() {
  if (!totpEnable.value.password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  totpEnable.value.loading = true
  try {
    const { data } = await securityTotpBegin<Security.TotpBeginResponse>(totpEnable.value.password)
    totpEnable.value.flowId = data.flowId
    totpEnable.value.secret = data.secret
    totpEnable.value.otpauthUrl = data.otpauthUrl
    totpEnable.value.password = ''
    totpEnable.value.step = 'verify'
  }
  catch (error) {
    apiRespErrMsg(error as Parameters<typeof apiRespErrMsg>[0])
  }
  finally {
    totpEnable.value.loading = false
  }
}

async function submitTotpVerify() {
  const code = normalizeCode(totpEnable.value.code)
  if (code.length !== 6) {
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

async function submitTotpDisable() {
  if (!totpDisable.value.password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  const code = normalizeCode(totpDisable.value.code)
  if (code.length !== 6) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }
  totpDisable.value.loading = true
  try {
    await securityTotpDisableApi(totpDisable.value.password, code)
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

async function submitPasskeyCreate() {
  if (passkeyCreate.value.step !== 'password')
    return
  if (!passkeyCreate.value.password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  const name = passkeyCreate.value.name.trim() || t('apps.userInfo.security.passkeyDefaultName')
  const totpCode = normalizeCode(passkeyCreate.value.totpCode)
  if (status.value.totpEnabled && totpCode.length !== 6) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }

  passkeyCreate.value.loading = true
  try {
    const { data } = await securityPasskeyBegin<Security.PublicKeyFlowResponse>(
      passkeyCreate.value.password,
      name,
      status.value.totpEnabled ? totpCode : undefined,
    )
    passkeyCreate.value.password = ''
    passkeyCreate.value.totpCode = ''

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

async function submitPasskeyDelete() {
  if (!passkeyDelete.value.password) {
    ms.warning(t('apps.userInfo.security.passwordRequired'))
    return
  }
  const totpCode = normalizeCode(passkeyDelete.value.totpCode)
  if (status.value.totpEnabled && totpCode.length !== 6) {
    ms.warning(t('apps.userInfo.security.codeInvalid'))
    return
  }

  passkeyDelete.value.loading = true
  try {
    await securityPasskeyDeleteApi(
      passkeyDelete.value.id,
      passkeyDelete.value.password,
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
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <SvgIcon icon="mdi:shield-key-outline" style="width:18px;height:18px;" />
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
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <SvgIcon icon="mdi:fingerprint" style="width:18px;height:18px;" />
          <span class="font-bold">{{ t('apps.userInfo.security.passkeyTitle') }}</span>
          <NTag size="small" :type="status.passkeys.length > 0 ? 'success' : 'default'">
            {{ t('apps.userInfo.security.passkeyCount', { count: status.passkeys.length }) }}
          </NTag>
        </div>
        <NButton size="small" text type="primary" :disabled="!status.passkeyAvailable" @click="openPasskeyCreate">
          {{ t('apps.userInfo.security.passkeyAdd') }}
        </NButton>
      </div>
      <div class="text-slate-500 text-xs mt-1">
        {{ status.passkeyAvailable ? t('apps.userInfo.security.passkeyDesc') : t('apps.userInfo.security.passkeyUnavailable') }}
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
      <div v-if="totpEnable.step === 'password'">
        <div class="text-slate-500 text-xs mb-2">
          {{ t('apps.userInfo.security.totpPasswordTip') }}
        </div>
        <NInput
          v-model:value="totpEnable.password" type="password" show-password-on="click"
          :placeholder="t('common.password')" @keydown.enter="submitTotpPassword"
        />
      </div>

      <div v-else>
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
          <div class="flex items-center justify-between gap-2">
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
          <NInput
            v-model:value="totpEnable.code" :maxlength="6" inputmode="numeric" :allow-input="onlyDigits"
            :placeholder="t('apps.userInfo.security.codePlaceholder')" @keydown.enter="submitTotpVerify"
          />
        </div>

        <div class="text-slate-500 text-xs mt-2">
          {{ t('apps.userInfo.security.totpReaddTip') }}
        </div>
      </div>

      <template #footer>
        <div class="float-right">
          <NButton
            size="small" type="primary" :loading="totpEnable.loading"
            @click="totpEnable.step === 'password' ? submitTotpPassword() : submitTotpVerify()"
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
      <div class="text-slate-500 text-xs mb-2">
        {{ t('apps.userInfo.security.totpDisableTip') }}
      </div>
      <NInput
        v-model:value="totpDisable.password" type="password" show-password-on="click"
        :placeholder="t('common.password')"
      />
      <div class="mt-2">
        <NInput
          v-model:value="totpDisable.code" :maxlength="6" inputmode="numeric" :allow-input="onlyDigits"
          :placeholder="t('apps.userInfo.security.codePlaceholder')" @keydown.enter="submitTotpDisable"
        />
      </div>
      <template #footer>
        <div class="float-right">
          <NButton size="small" type="error" :loading="totpDisable.loading" @click="submitTotpDisable">
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
      <div v-if="passkeyCreate.step === 'password'">
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
        <NInput
          v-model:value="passkeyCreate.password" type="password" show-password-on="click"
          :placeholder="t('common.password')"
        />
        <div v-if="status.totpEnabled" class="mt-2">
          <div class="text-slate-500 text-xs mb-1">
            {{ t('apps.userInfo.security.codeLabel') }}
          </div>
          <NInput
            v-model:value="passkeyCreate.totpCode" :maxlength="6" inputmode="numeric" :allow-input="onlyDigits"
            :placeholder="t('apps.userInfo.security.codePlaceholder')"
          />
        </div>
      </div>
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
            :disabled="passkeyCreate.step === 'waiting'" @click="submitPasskeyCreate"
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
      <div class="text-slate-500 text-xs mb-2">
        {{ t('apps.userInfo.security.passkeyDeleteTip', { name: passkeyDelete.name || t('apps.userInfo.security.passkeyDefaultName') }) }}
      </div>
      <NInput
        v-model:value="passkeyDelete.password" type="password" show-password-on="click"
        :placeholder="t('common.password')"
      />
      <div v-if="status.totpEnabled" class="mt-2">
        <div class="text-slate-500 text-xs mb-1">
          {{ t('apps.userInfo.security.codeLabel') }}
        </div>
        <NInput
          v-model:value="passkeyDelete.totpCode" :maxlength="6" inputmode="numeric" :allow-input="onlyDigits"
          :placeholder="t('apps.userInfo.security.codePlaceholder')"
        />
      </div>
      <template #footer>
        <div class="float-right">
          <NButton size="small" type="error" :loading="passkeyDelete.loading" @click="submitPasskeyDelete">
            {{ t('common.delete') }}
          </NButton>
        </div>
      </template>
    </RoundCardModal>
  </NCard>
</template>
