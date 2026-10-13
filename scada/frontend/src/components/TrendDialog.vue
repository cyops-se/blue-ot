<template>
  <v-dialog :model-value="!!tagId" max-width="800" @update:model-value="!$event && $emit('close')">
    <v-card v-if="tagId">
      <v-card-title class="d-flex align-center">
        <v-icon icon="mdi-chart-line" class="mr-2" />
        {{ tag ? tag.name : tagId }}
        <v-spacer />
        <span class="mono text-h6">{{ current }}</span>
      </v-card-title>
      <v-card-subtitle>{{ tagId }} · {{ tag ? `${tag.area} ${tag.address} (${tag.dataType})` : '' }}</v-card-subtitle>
      <v-card-text>
        <v-btn-toggle v-model="minutes" density="compact" variant="outlined" mandatory class="mb-2">
          <v-btn :value="5">5 min</v-btn>
          <v-btn :value="10">10 min</v-btn>
          <v-btn :value="30">30 min</v-btn>
          <v-btn :value="60">60 min</v-btn>
        </v-btn-toggle>
        <TrendChart :samples="samples" :decimals="tag ? tag.decimals : 2" :unit="tag ? tag.unit : ''" />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="$emit('close')">Stäng</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import TrendChart from './TrendChart.vue'
import { api } from '../lib/api'
import { useConfig } from '../stores/config'
import { useLive } from '../stores/live'
import { formatValue } from '../lib/symbols'

const props = defineProps({ tagId: { type: String, default: null } })
defineEmits(['close'])

const config = useConfig()
const live = useLive()
const minutes = ref(10)
const samples = ref([])
let timer = null

const tag = computed(() => config.tagMap[props.tagId])
const current = computed(() => `${formatValue(live.values[props.tagId], tag.value)} ${tag.value ? tag.value.unit : ''}`)

async function load() {
  if (!props.tagId) return
  try {
    samples.value = await api.history(props.tagId, minutes.value)
  } catch {
    samples.value = []
  }
}

watch(
  () => [props.tagId, minutes.value],
  () => {
    clearInterval(timer)
    samples.value = []
    if (props.tagId) {
      load()
      timer = setInterval(load, 2000)
    }
  },
  { immediate: true },
)
onUnmounted(() => clearInterval(timer))
</script>
