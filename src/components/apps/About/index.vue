<script setup lang="ts">
import { NDivider, NGradientText, NTag } from 'naive-ui'
import { onMounted, ref } from 'vue'
import { get } from '@/api/system/about'
import srcSvglogo from '@/assets/logo.svg'
import srcGithub from '@/assets/about_image/github.png'

interface Version {
  versionName: string
  versionCode: number
}

const projectUrl = 'https://github.com/Seameee/sun-panel'
const tagsUrl = `${projectUrl}/tags`
const issuesUrl = `${projectUrl}/issues`
const versionName = ref('')
const frontVersion = import.meta.env.VITE_APP_VERSION || 'unknown'

onMounted(() => {
  get<Version>().then((res) => {
    if (res.code === 0)
      versionName.value = res.data.versionName
  })
})
</script>

<template>
  <div class="pt-5">
    <div class="flex flex-col items-center justify-center">
      <img :src="srcSvglogo" width="100" height="100" alt="">
      <div class="text-3xl font-semibold">
        {{ $t('common.appName') }}
      </div>
      <div class="text-xl">
        <NGradientText type="info">
          <a :href="tagsUrl" class="font-semibold" :title="$t('apps.about.viewUpdateLog')" target="_blank" rel="noopener noreferrer">v{{ versionName }}</a>
        </NGradientText>
      </div>
      <div class="mt-2">
        <a :href="tagsUrl" target="_blank" rel="noopener noreferrer" class="link">{{ $t('apps.about.checkUpdate') }}</a>
      </div>
    </div>

    <NDivider style="margin:10px 0">
      •
    </NDivider>
    <div class="flex flex-col items-center justify-center text-base">
      <div>
        {{ $t('apps.about.author') }}<a href="https://github.com/hslr-s" target="_blank" rel="noopener noreferrer" class="link">红烧猎人</a>
      </div>
      <div>
        {{ $t('apps.about.issue') }}<a :href="issuesUrl" target="_blank" rel="noopener noreferrer" class="link">GitHub Issues</a>
      </div>
      <div class="flex items-center mt-[10px]">
        <img class="w-[20px] h-[20px] mr-[5px]" :src="srcGithub" alt="">
        <a :href="projectUrl" target="_blank" rel="noopener noreferrer" class="link">GitHub</a>
      </div>

      <div class="mt-5">
        <NTag :bordered="false" size="small">
          {{ $t('apps.about.frontVersionText') }}: FV-{{ frontVersion }}
        </NTag>
      </div>
    </div>
  </div>
</template>

<style>
.link{
    color:rgb(0, 89, 255)
}
</style>
