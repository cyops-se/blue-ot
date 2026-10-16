<template>
  <div class="runtime">
    <div class="toolbar d-flex align-center px-3">
      <v-select
        :model-value="id"
        :items="views"
        item-title="name"
        item-value="id"
        density="compact"
        variant="plain"
        style="max-width: 280px"
        @update:model-value="(v) => router.push(`/views/${v}`)"
      />
      <v-spacer />
      <v-btn size="small" variant="text" prepend-icon="mdi-pencil" :to="`/views/${id}/edit`">Redigera</v-btn>
    </div>
    <div class="stage">
      <HmiCanvas v-if="view" :view="view" @activate="activate" />
      <v-progress-circular v-else-if="!error" indeterminate class="ma-8" />
      <v-alert v-else type="error" class="ma-4">{{ error }}</v-alert>
    </div>
    <CommandDialog :el="commandEl" @close="commandEl = null" />
    <TrendDialog :tag-id="trendTag" @close="trendTag = null" />
  </div>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import HmiCanvas from '../components/HmiCanvas.vue'
import CommandDialog from '../components/CommandDialog.vue'
import TrendDialog from '../components/TrendDialog.vue'
import { api } from '../lib/api'

const props = defineProps({ id: { type: String, required: true } })
const router = useRouter()
const view = ref(null)
const views = ref([])
const error = ref('')
const commandEl = ref(null)
const trendTag = ref(null)

async function load() {
  view.value = null
  error.value = ''
  try {
    view.value = await api.view(props.id)
  } catch (e) {
    error.value = e.message
  }
}

function activate(el) {
  switch (el.type) {
    case 'breaker':
    case 'disconnector':
    case 'toggle':
    case 'pump':
    case 'valve':
    case 'button':
      commandEl.value = el
      break
    case 'measurement':
    case 'bar':
      trendTag.value = el.props.tag
      break
    case 'link':
      if (el.props.view) router.push(`/views/${el.props.view}`)
      break
  }
}

watch(() => props.id, load)
onMounted(async () => {
  load()
  views.value = await api.views()
})
</script>

<style scoped>
.runtime { height: calc(100vh - 48px); display: flex; flex-direction: column; }
.toolbar { height: 44px; border-bottom: 1px solid rgba(255, 255, 255, 0.08); }
.stage { flex: 1; min-height: 0; padding: 8px; }
</style>
