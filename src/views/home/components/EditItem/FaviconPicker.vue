<script setup lang="ts">
import { NButton, NEmpty, NGrid, NGridItem, NSpin } from 'naive-ui'
import { computed, nextTick, ref, watch } from 'vue'
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

const rootRef = ref<HTMLElement | null>(null)

const show = computed({
  get: () => props.visible,
  set: (visible: boolean) => emit('update:visible', visible),
})

// 面板展开后滚动到可见位置，避免用户自己往下找
watch(() => props.visible, async (visible) => {
  if (!visible)
    return
  await nextTick()
  rootRef.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
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
  <!-- 内嵌在图标预览下方，随表单一起滚动，不用再滚到弹窗底部 -->
  <div v-if="show" ref="rootRef" class="favicon-picker">
    <div class="favicon-picker-head">
      <span class="favicon-picker-title">{{ t('iconItem.faviconPickTitle') }}</span>
      <span class="favicon-picker-tip">{{ t('iconItem.faviconPickTip') }}</span>
    </div>

    <div v-if="loading" class="flex justify-center py-4">
      <NSpin size="small" />
    </div>

    <NEmpty v-else-if="options.length === 0" size="small" :description="t('iconItem.geticonEmpty')" />

    <NGrid v-else cols="3 400:4 640:5" :x-gap="8" :y-gap="8">
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

    <div class="favicon-picker-foot">
      <NButton size="tiny" quaternary @click="show = false">
        {{ t('common.cancel') }}
      </NButton>
    </div>
  </div>
</template>

<style scoped>
.favicon-picker {
  margin-top: 8px;
  padding: 10px;
  border: 1px solid rgba(148, 163, 184, 0.45);
  border-radius: 10px;
  background: rgba(148, 163, 184, 0.06);
}

.favicon-picker-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 8px;
}

.favicon-picker-title {
  font-size: 13px;
  font-weight: 500;
  color: #475569;
}

.favicon-picker-tip {
  font-size: 12px;
  color: #94a3b8;
}

.favicon-picker-foot {
  margin-top: 6px;
  text-align: right;
}

.transparent-grid {
  background-image: linear-gradient(45deg, #fff 25%, transparent 25%, transparent 75%, #fff 75%),
                    linear-gradient(45deg, #fff 25%, transparent 25%, transparent 75%, #fff 75%);
  background-size: 16px 16px;
  background-position: 0 0, 8px 8px;
}

.favicon-pick-card {
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: border-color 0.15s, transform 0.15s;
}

.favicon-pick-card:hover {
  border-color: #18a058;
  transform: translateY(-1px);
}

.favicon-pick-thumb {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 6px;
}

.favicon-pick-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
}

.favicon-pick-size {
  padding: 2px 6px;
  font-size: 11px;
  color: #64748b;
  text-align: center;
  border-top: 1px solid rgba(148, 163, 184, 0.25);
}
</style>
