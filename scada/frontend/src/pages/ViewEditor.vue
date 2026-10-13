<template>
  <div class="editor">
    <!-- toolbar -->
    <div class="toolbar d-flex align-center px-2 ga-1">
      <v-btn size="small" color="primary" variant="flat" prepend-icon="mdi-content-save" :loading="saving" @click="save">
        Spara<span v-if="dirty">*</span>
      </v-btn>
      <v-btn size="small" variant="text" icon="mdi-undo" title="Ångra (Ctrl+Z)" :disabled="!undoStack.length" @click="undo" />
      <v-divider vertical class="mx-1" />
      <v-btn size="small" variant="text" icon="mdi-content-copy" title="Duplicera (Ctrl+D)" :disabled="!selectedId" @click="duplicate" />
      <v-btn size="small" variant="text" icon="mdi-arrange-bring-forward" title="Flytta fram" :disabled="!selectedId" @click="reorder(1)" />
      <v-btn size="small" variant="text" icon="mdi-arrange-send-backward" title="Flytta bak" :disabled="!selectedId" @click="reorder(-1)" />
      <v-btn size="small" variant="text" icon="mdi-delete" title="Ta bort (Del)" :disabled="!selectedId" @click="removeSelected" />
      <v-divider vertical class="mx-1" />
      <v-btn size="small" variant="text" icon="mdi-magnify-minus" @click="zoom = Math.max(0.25, zoom - 0.1)" />
      <span class="text-caption mono" style="width: 44px; text-align: center">{{ Math.round(zoom * 100) }}%</span>
      <v-btn size="small" variant="text" icon="mdi-magnify-plus" @click="zoom = Math.min(3, zoom + 0.1)" />
      <v-spacer />
      <v-btn size="small" variant="text" prepend-icon="mdi-play" @click="openRuntime">Driftläge</v-btn>
    </div>

    <div class="body">
      <!-- palette -->
      <div class="palette">
        <template v-for="g in groups" :key="g">
          <div class="text-overline px-3 pt-2">{{ g }}</div>
          <div class="palette-grid px-2">
            <div
              v-for="(def, type) in byGroup(g)"
              :key="type"
              class="palette-item"
              draggable="true"
              :title="def.label"
              @dragstart="(e) => e.dataTransfer.setData('text/hmi-type', type)"
              @dblclick="addAtCenter(type)"
            >
              <v-icon :icon="def.icon" size="22" />
              <div class="text-caption">{{ def.label }}</div>
            </div>
          </div>
        </template>
      </div>

      <!-- canvas -->
      <div class="canvas-wrap" ref="wrap">
        <HmiCanvas
          v-if="view"
          :view="view"
          editable
          :zoom="zoom"
          :selected-id="selectedId"
          @select="selectedId = $event"
          @before-change="snapshot"
          @drop="addElement"
        />
      </div>

      <!-- properties -->
      <div class="props">
        <PropertiesPanel v-if="view" :el="selected" :view="view" @before-change="snapshot" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, onUnmounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import HmiCanvas from '../components/HmiCanvas.vue'
import PropertiesPanel from '../components/PropertiesPanel.vue'
import { catalog, groups, newElement } from '../lib/symbols'
import { api } from '../lib/api'
import { useConfig } from '../stores/config'

const props = defineProps({ id: { type: String, required: true } })
const notify = inject('notify')
const router = useRouter()
const config = useConfig()

const view = ref(null)
const selectedId = ref(null)
const zoom = ref(1)
const saving = ref(false)
const dirty = ref(false)
const undoStack = ref([])
const wrap = ref(null)
let lastSnapshot = 0

const selected = computed(() => (view.value ? view.value.elements.find((e) => e.id === selectedId.value) || null : null))
const byGroup = (g) => Object.fromEntries(Object.entries(catalog).filter(([, d]) => d.group === g))

async function load() {
  view.value = await api.view(props.id)
  undoStack.value = []
  dirty.value = false
  // fit to the available width
  requestAnimationFrame(() => {
    if (wrap.value) zoom.value = Math.min(1, Math.max(0.25, Math.floor(((wrap.value.clientWidth - 40) / view.value.width) * 10) / 10))
  })
}

// snapshot pushes an undo entry; rapid successive edits (typing, dragging)
// are merged into one entry.
function snapshot() {
  const now = Date.now()
  dirty.value = true
  if (now - lastSnapshot < 800) {
    lastSnapshot = now
    return
  }
  lastSnapshot = now
  undoStack.value.push(JSON.stringify(view.value))
  if (undoStack.value.length > 100) undoStack.value.shift()
}

function undo() {
  const s = undoStack.value.pop()
  if (!s) return
  view.value = JSON.parse(s)
  lastSnapshot = 0
  if (!view.value.elements.some((e) => e.id === selectedId.value)) selectedId.value = null
}

function addElement(type, x, y) {
  lastSnapshot = 0
  snapshot()
  const el = newElement(type, x, y)
  view.value.elements.push(el)
  selectedId.value = el.id
}

function addAtCenter(type) {
  const g = view.value.grid || 10
  addElement(type, Math.round(view.value.width / 2 / g) * g, Math.round(view.value.height / 2 / g) * g)
}

function duplicate() {
  if (!selected.value) return
  lastSnapshot = 0
  snapshot()
  const copy = JSON.parse(JSON.stringify(selected.value))
  copy.id = newElement(copy.type, 0, 0).id
  copy.x += view.value.grid * 2
  copy.y += view.value.grid * 2
  view.value.elements.push(copy)
  selectedId.value = copy.id
}

function removeSelected() {
  if (!selected.value) return
  lastSnapshot = 0
  snapshot()
  view.value.elements = view.value.elements.filter((e) => e.id !== selectedId.value)
  selectedId.value = null
}

function reorder(dir) {
  const els = view.value.elements
  const i = els.findIndex((e) => e.id === selectedId.value)
  const j = i + dir
  if (i < 0 || j < 0 || j >= els.length) return
  lastSnapshot = 0
  snapshot()
  ;[els[i], els[j]] = [els[j], els[i]]
}

async function save() {
  saving.value = true
  try {
    view.value = await api.saveView(view.value)
    dirty.value = false
    notify('Vyn sparad')
  } catch (e) {
    notify(`Kunde inte spara: ${e.message}`, 'error')
  } finally {
    saving.value = false
  }
}

function openRuntime() {
  router.push(`/views/${props.id}`)
}

function onKey(e) {
  const tag = (e.target.tagName || '').toLowerCase()
  const typing = tag === 'input' || tag === 'textarea' || e.target.isContentEditable
  if (e.ctrlKey && e.key.toLowerCase() === 's') {
    e.preventDefault()
    save()
    return
  }
  if (typing) return
  if (e.ctrlKey && e.key.toLowerCase() === 'z') {
    e.preventDefault()
    undo()
  } else if (e.ctrlKey && e.key.toLowerCase() === 'd') {
    e.preventDefault()
    duplicate()
  } else if (e.key === 'Delete') {
    removeSelected()
  } else if (e.key === 'Escape') {
    selectedId.value = null
  } else if (selected.value && e.key.startsWith('Arrow')) {
    e.preventDefault()
    const step = e.shiftKey ? view.value.grid * 5 : e.altKey ? 1 : view.value.grid
    snapshot()
    if (e.key === 'ArrowLeft') selected.value.x -= step
    if (e.key === 'ArrowRight') selected.value.x += step
    if (e.key === 'ArrowUp') selected.value.y -= step
    if (e.key === 'ArrowDown') selected.value.y += step
  }
}

onBeforeRouteLeave(() => !dirty.value || confirm('Det finns osparade ändringar. Lämna ändå?'))
const beforeUnload = (e) => {
  if (dirty.value) e.preventDefault()
}

watch(() => props.id, load)
onMounted(() => {
  load()
  config.load()
  window.addEventListener('keydown', onKey)
  window.addEventListener('beforeunload', beforeUnload)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('beforeunload', beforeUnload)
})
</script>

<style scoped>
.editor { height: calc(100vh - 48px); display: flex; flex-direction: column; }
.toolbar { height: 44px; border-bottom: 1px solid rgba(255, 255, 255, 0.08); flex: none; }
.body { flex: 1; display: flex; min-height: 0; }
.palette { width: 220px; flex: none; overflow-y: auto; border-right: 1px solid rgba(255, 255, 255, 0.08); }
.palette-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; }
.palette-item {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  height: 64px; border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 6px;
  cursor: grab; text-align: center; line-height: 1.1; padding: 4px; overflow: hidden; word-break: break-word;
}
.palette-item:hover { background: rgba(61, 139, 253, 0.12); border-color: #3d8bfd; }
.canvas-wrap { flex: 1; overflow: auto; padding: 20px; background: #0a0d11; }
.props { width: 330px; flex: none; overflow-y: auto; border-left: 1px solid rgba(255, 255, 255, 0.08); }
</style>
