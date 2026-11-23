<script setup lang="ts">
import { NDivider, NGradientText } from 'naive-ui'
import srcSvglogo from '@/assets/logo.svg'
import srcGithub from '@/assets/about_image/github.png'

const versionName = '1.10'
</script>

<template>
  <div class="pt-5">
    <div class="flex flex-col items-center justify-center">
      <img :src="srcSvglogo" width="100" height="100" alt="">
      <div class="text-3xl font-semibold">
        XH-Panel
      </div>
      <div class="text-xl">
        <NGradientText type="info">
          <a href="https://github.com/laozixh/XH-Panel/releases" class="font-semibold" :title="$t('apps.about.viewUpdateLog')" target="_blank">Ver: {{ versionName }}</a>
        </NGradientText>
      </div>
      <div class="mt-2">
        <a href="https://github.com/laozixh/XH-Panel/releases" target="_blank" class="link">{{ $t('apps.about.checkUpdate') }}</a>
      </div>
    </div>

    <NDivider style="margin:10px 0">
      •
    </NDivider>
    <div class="flex flex-col items-center justify-center text-base">
      <div class="flex mt-[10px] flex-wrap justify-center">
        <div class="flex items-center mx-[10px]">
          <img class="w-[20px] h-[20px] mr-[5px]" :src="srcGithub" alt="">
          <a href="https://github.com/laozixh/XH-Panel" target="_blank" class="link">Github</a>
        </div>
      </div>

      <div class="mt-4 text-sm text-gray-500">
        <a href="https://github.com/hslr-s/sun-panel" target="_blank" class="link">本项目基于 Sun-Panel 开源版 v1.3.0 二次开发</a>
      </div>
    </div>
  </div>
</template>

<style>
.link{
    color:rgb(0, 89, 255)
}
</style>
