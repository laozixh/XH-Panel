<script setup lang="ts">
import { NButton, NEmpty, NGrid, NGridItem, NModal, NSpin } from 'naive-ui'
import { computed } from 'vue'
import { t } from '@/locales'

export interface FaviconOption {
  sourceUrl: string
  preview: string
  width: number
  height: number
}

interface Props {
  visible: boolean
  options: FaviconOption[]
  loading?: boolean
}

const props = withDefaults(defineProps<Props>(), { loading: false })
const emit = defineEmits<{
  (e: 'update:visible', visible: boolean): void
  (e: 'select', sourceUrl: string): void
}>()

const show = computed({
  get: () => props.visible,
  set: (visible: boolean) => emit('update:visible', visible),
})

function sizeText(option: FaviconOption) {
  if (!option.width || !option.height)
    return t('iconItem.faviconVector')
  return `${option.width}×${option.height}`
}

function handleSelect(option: FaviconOption) {
  emit('select', option.sourceUrl)
  show.value = false
}
</script>

<template>
  <NModal v-model:show="show" preset="card" size="small" style="width: 640px;border-radius: 1rem;" :title="t('iconItem.faviconPickTitle')">
    <div class="min-h-[180px]">
      <div class="mb-3 text-slate-500 dark:text-slate-400 text-sm">
        {{ t('iconItem.faviconPickTip') }}
      </div>

      <div v-if="loading" class="flex justify-center py-6">
        <NSpin size="small" />
      </div>

      <NEmpty v-else-if="options.length === 0" :description="t('iconItem.geticonEmpty')" />

      <NGrid v-else cols="2 400:3 640:4" :x-gap="10" :y-gap="10">
        <NGridItem v-for="(option, index) in options" :key="`favicon-${index}`">
          <div class="favicon-pick-card" @click="handleSelect(option)">
            <div class="favicon-pick-thumb transparent-grid">
              <img :src="option.preview" class="favicon-pick-img" alt="favicon">
            </div>
            <div class="favicon-pick-size">
              {{ sizeText(option) }}
            </div>
          </div>
        </NGridItem>
      </NGrid>
    </div>

    <template #footer>
      <NButton style="float: right;" @click="show = false">
        {{ t('common.cancel') }}
      </NButton>
    </template>
  </NModal>
</template>

<style scoped>
.transparent-grid {
  background-image: linear-gradient(45deg, #fff 25%, transparent 25%, transparent 75%, #fff 75%),
                    linear-gradient(45deg, #fff 25%, transparent 25%, transparent 75%, #fff 75%);
  background-size: 16px 16px;
  background-position: 0 0, 8px 8px;
}

.favicon-pick-card {
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: 10px;
  overflow: hidden;
  cursor: pointer;
  transition: border-color 0.15s, transform 0.15s;
}

.favicon-pick-card:hover {
  border-color: #18a058;
  transform: translateY(-1px);
}

.favicon-pick-thumb {
  height: 88px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 8px;
}

.favicon-pick-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
}

.favicon-pick-size {
  padding: 4px 8px;
  font-size: 12px;
  color: #64748b;
  text-align: center;
  border-top: 1px solid rgba(148, 163, 184, 0.25);
}
</style>
