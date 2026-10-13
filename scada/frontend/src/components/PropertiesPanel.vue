<template>
  <div v-if="el" class="pa-3">
    <div class="d-flex align-center mb-3">
      <v-icon :icon="def.icon" class="mr-2" />
      <strong>{{ def.label }}</strong>
      <v-spacer />
      <span class="text-caption text-medium-emphasis mono">{{ el.id }}</span>
    </div>

    <div class="text-overline">Position</div>
    <v-row dense>
      <v-col cols="6"><v-text-field :model-value="el.x" label="X" type="number" @update:model-value="setPos('x', $event)" /></v-col>
      <v-col cols="6"><v-text-field :model-value="el.y" label="Y" type="number" @update:model-value="setPos('y', $event)" /></v-col>
      <v-col cols="6"><v-text-field :model-value="el.w" :label="def.line ? 'ΔX' : 'Bredd'" type="number" @update:model-value="setPos('w', $event)" /></v-col>
      <v-col cols="6"><v-text-field :model-value="el.h" :label="def.line ? 'ΔY' : 'Höjd'" type="number" @update:model-value="setPos('h', $event)" /></v-col>
      <v-col v-if="!def.line" cols="12">
        <v-select :model-value="el.rot || 0" :items="[0, 90, 180, 270]" label="Rotation (°)" @update:model-value="setPos('rot', $event)" />
      </v-col>
    </v-row>

    <template v-for="(f, i) in def.fields" :key="f.key || 's' + i">
      <div v-if="f.section" class="text-overline mt-2">{{ f.section }}</div>

      <v-text-field v-else-if="f.type === 'text'" class="mt-2" :model-value="el.props[f.key]" :label="f.label" @update:model-value="set(f.key, $event)" />

      <v-text-field
        v-else-if="f.type === 'number'" class="mt-2" type="number" :model-value="el.props[f.key]" :label="f.label"
        @update:model-value="set(f.key, $event === '' ? '' : Number($event))"
      />

      <v-switch v-else-if="f.type === 'bool'" class="mt-1" :model-value="!!el.props[f.key]" :label="f.label" @update:model-value="set(f.key, $event)" />

      <v-select v-else-if="f.type === 'select'" class="mt-2" :model-value="el.props[f.key]" :items="f.options" :label="f.label" @update:model-value="set(f.key, $event)" />

      <ColorField v-else-if="f.type === 'color'" class="mt-2" :model-value="el.props[f.key]" :label="f.label" @update:model-value="set(f.key, $event)" />

      <v-autocomplete
        v-else-if="f.type === 'tag'" class="mt-2" clearable
        :model-value="el.props[f.key] || null"
        :items="tagItems(f)"
        :label="f.label"
        @update:model-value="setTag(f.key, $event)"
      >
        <template #item="{ props: ip, item }">
          <v-list-item v-bind="ip" :subtitle="item.raw.subtitle" />
        </template>
      </v-autocomplete>

      <v-select
        v-else-if="f.type === 'view'" class="mt-2" :model-value="el.props[f.key]" :items="views" item-title="name" item-value="id"
        :label="f.label" @update:model-value="set(f.key, $event)"
      />

      <div v-else-if="f.type === 'image'" class="mt-2">
        <v-select :model-value="el.props[f.key]" :items="images" :label="f.label" clearable @update:model-value="set(f.key, $event || '')" />
        <v-btn class="mt-2" size="small" variant="tonal" prepend-icon="mdi-upload" block @click="$event.currentTarget.nextElementSibling.click()">Ladda upp bild</v-btn>
        <input type="file" accept="image/*" hidden @change="upload($event, f.key)" />
      </div>

      <div v-else-if="f.type === 'rules'" class="mt-2">
        <div class="text-caption text-medium-emphasis mb-1">{{ f.label }} (första matchande regel används)</div>
        <div v-for="(r, ri) in el.props[f.key]" :key="ri" class="d-flex ga-1 mb-1 align-center">
          <v-select :model-value="r.op" :items="ruleOps" style="max-width: 76px" @update:model-value="setRule(f.key, ri, 'op', $event)" />
          <v-text-field :model-value="r.value" type="number" style="max-width: 80px" @update:model-value="setRule(f.key, ri, 'value', Number($event))" />
          <ColorField :model-value="r.color" @update:model-value="setRule(f.key, ri, 'color', $event)" />
          <v-btn icon="mdi-close" size="x-small" variant="text" @click="removeRule(f.key, ri)" />
        </div>
        <v-btn size="small" variant="text" prepend-icon="mdi-plus" @click="addRule(f.key)">Lägg till regel</v-btn>
      </div>
    </template>
  </div>

  <div v-else class="pa-3">
    <div class="text-overline">Vy</div>
    <v-text-field class="mt-2" :model-value="view.name" label="Namn" @update:model-value="setView('name', $event)" />
    <v-row dense class="mt-1">
      <v-col cols="6"><v-text-field :model-value="view.width" label="Bredd" type="number" @update:model-value="setView('width', Number($event))" /></v-col>
      <v-col cols="6"><v-text-field :model-value="view.height" label="Höjd" type="number" @update:model-value="setView('height', Number($event))" /></v-col>
    </v-row>
    <v-text-field class="mt-2" :model-value="view.grid" label="Rutnät (px)" type="number" @update:model-value="setView('grid', Math.max(1, Number($event)))" />
    <ColorField class="mt-2" :model-value="view.background" label="Bakgrund" @update:model-value="setView('background', $event)" />
    <v-alert class="mt-4" variant="tonal" density="compact" type="info">
      Dra symboler från paletten till ritytan. Markera en symbol för att konfigurera taggar och utseende.
      <div class="text-caption mt-1">Del = ta bort · Ctrl+D = duplicera · Pilar = flytta · Ctrl+Z = ångra · Ctrl+S = spara</div>
    </v-alert>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import ColorField from './ColorField.vue'
import { catalog, ruleOps } from '../lib/symbols'
import { useConfig } from '../stores/config'
import { api } from '../lib/api'

const props = defineProps({
  el: { type: Object, default: null },
  view: { type: Object, required: true },
})
const emit = defineEmits(['before-change'])

const config = useConfig()
const images = ref([])
const views = ref([])
const def = computed(() => (props.el ? catalog[props.el.type] : null))

function tagItems(f) {
  return config.tags
    .filter((t) => !f.writable || t.writable)
    .map((t) => ({ title: t.id, value: t.id, subtitle: `${t.name} · ${t.area} ${t.address}${t.unit ? ' · ' + t.unit : ''}` }))
}

function set(key, value) {
  emit('before-change')
  props.el.props[key] = value
}

function setTag(key, value) {
  set(key, value || '')
  const t = config.tagMap[value]
  if (t && props.el.type === 'measurement' && key === 'tag' && !props.el.props.label) props.el.props.label = t.name
}

function setPos(key, value) {
  emit('before-change')
  props.el[key] = Number(value) || 0
}

function setView(key, value) {
  emit('before-change')
  props.view[key] = value
}

function setRule(key, i, field, value) {
  emit('before-change')
  props.el.props[key][i][field] = value
}
function addRule(key) {
  emit('before-change')
  if (!Array.isArray(props.el.props[key])) props.el.props[key] = []
  props.el.props[key].push({ op: '==', value: 1, color: '#ff4d4d' })
}
function removeRule(key, i) {
  emit('before-change')
  props.el.props[key].splice(i, 1)
}

async function upload(evt, key) {
  const file = evt.target.files[0]
  if (!file) return
  const res = await api.uploadImage(file)
  images.value = await api.images()
  set(key, res.url)
  evt.target.value = ''
}

onMounted(async () => {
  images.value = await api.images().catch(() => [])
  views.value = await api.views().catch(() => [])
})
</script>
