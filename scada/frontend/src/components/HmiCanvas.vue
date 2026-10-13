<template>
  <svg
    ref="svg"
    class="hmi-svg"
    :viewBox="`0 0 ${view.width} ${view.height}`"
    :width="editable ? view.width * zoom : '100%'"
    :height="editable ? view.height * zoom : '100%'"
    preserveAspectRatio="xMidYMid meet"
    @pointerdown="onBackgroundDown"
    @pointermove="onMove"
    @pointerup="onUp"
    @pointercancel="onUp"
    @dragover.prevent
    @drop.prevent="onDrop"
  >
    <defs>
      <pattern v-if="editable" id="hmi-grid" :width="view.grid" :height="view.grid" patternUnits="userSpaceOnUse">
        <path :d="`M ${view.grid} 0 L 0 0 0 ${view.grid}`" fill="none" stroke="#ffffff" stroke-opacity="0.06" stroke-width="1" />
      </pattern>
    </defs>
    <rect :width="view.width" :height="view.height" :fill="view.background || '#0d1117'" />
    <rect v-if="editable" :width="view.width" :height="view.height" fill="url(#hmi-grid)" pointer-events="none" />

    <g
      v-for="el in view.elements"
      :key="el.id"
      :transform="transform(el)"
      @pointerdown="onElementDown($event, el)"
      :style="editable ? 'cursor: move' : null"
    >
      <HmiElement :el="el" :runtime="!editable" @activate="$emit('activate', $event)" />
    </g>

    <!-- selection overlay -->
    <g v-if="editable && selected" :transform="transform(selected)" pointer-events="all">
      <template v-if="isLine(selected)">
        <line x1="0" y1="0" :x2="selected.w" :y2="selected.h" stroke="#3d8bfd" stroke-width="1" stroke-dasharray="4 3" pointer-events="none" />
        <circle cx="0" cy="0" r="6" class="handle" @pointerdown.stop="onHandleDown($event, 'start')" />
        <circle :cx="selected.w" :cy="selected.h" r="6" class="handle" @pointerdown.stop="onHandleDown($event, 'end')" />
      </template>
      <template v-else>
        <rect x="-3" y="-3" :width="selected.w + 6" :height="selected.h + 6" fill="none" stroke="#3d8bfd" stroke-width="1" stroke-dasharray="4 3" pointer-events="none" />
        <rect :x="selected.w - 4" :y="selected.h - 4" width="10" height="10" class="handle" style="cursor: nwse-resize" @pointerdown.stop="onHandleDown($event, 'se')" />
      </template>
    </g>
  </svg>
</template>

<script setup>
import { computed, ref } from 'vue'
import HmiElement from './HmiElement.vue'
import { catalog } from '../lib/symbols'

const props = defineProps({
  view: { type: Object, required: true },
  editable: { type: Boolean, default: false },
  selectedId: { type: String, default: null },
  zoom: { type: Number, default: 1 },
})
const emit = defineEmits(['activate', 'select', 'before-change', 'drop'])

const svg = ref(null)
const selected = computed(() => props.view.elements.find((e) => e.id === props.selectedId) || null)
const isLine = (el) => !!(catalog[el.type] && catalog[el.type].line)

function transform(el) {
  const r = el.rot ? ` rotate(${el.rot} ${el.w / 2} ${el.h / 2})` : ''
  return `translate(${el.x} ${el.y})${r}`
}

function toSvg(evt) {
  const pt = svg.value.createSVGPoint()
  pt.x = evt.clientX
  pt.y = evt.clientY
  return pt.matrixTransform(svg.value.getScreenCTM().inverse())
}

const snap = (v) => {
  const g = props.view.grid || 1
  return Math.round(v / g) * g
}

// rotate a delta vector into the element's local coordinate system
function toLocal(el, dx, dy) {
  const a = (-(el.rot || 0) * Math.PI) / 180
  return { dx: dx * Math.cos(a) - dy * Math.sin(a), dy: dx * Math.sin(a) + dy * Math.cos(a) }
}

let drag = null

function onBackgroundDown() {
  if (props.editable) emit('select', null)
}

function onElementDown(evt, el) {
  if (!props.editable) return
  evt.stopPropagation()
  emit('select', el.id)
  const p = toSvg(evt)
  drag = { mode: 'move', el, p0: p, start: { x: el.x, y: el.y, w: el.w, h: el.h }, changed: false }
  svg.value.setPointerCapture(evt.pointerId)
}

function onHandleDown(evt, mode) {
  const el = selected.value
  const p = toSvg(evt)
  drag = { mode, el, p0: p, start: { x: el.x, y: el.y, w: el.w, h: el.h }, changed: false }
  svg.value.setPointerCapture(evt.pointerId)
}

function onMove(evt) {
  if (!drag) return
  const p = toSvg(evt)
  const dx = p.x - drag.p0.x
  const dy = p.y - drag.p0.y
  if (!drag.changed) {
    if (Math.abs(dx) < 2 && Math.abs(dy) < 2) return
    emit('before-change')
    drag.changed = true
  }
  const { el, start } = drag
  switch (drag.mode) {
    case 'move':
      el.x = snap(start.x + dx)
      el.y = snap(start.y + dy)
      break
    case 'se': {
      const l = toLocal(el, dx, dy)
      el.w = Math.max(props.view.grid || 4, snap(start.w + l.dx))
      el.h = Math.max(props.view.grid || 4, snap(start.h + l.dy))
      break
    }
    case 'start': {
      const nx = snap(start.x + dx)
      const ny = snap(start.y + dy)
      el.w = start.x + start.w - nx
      el.h = start.y + start.h - ny
      el.x = nx
      el.y = ny
      break
    }
    case 'end':
      el.w = snap(start.x + start.w + dx) - start.x
      el.h = snap(start.y + start.h + dy) - start.y
      break
  }
}

function onUp() {
  drag = null
}

function onDrop(evt) {
  const type = evt.dataTransfer.getData('text/hmi-type')
  if (!type || !props.editable) return
  const p = toSvg(evt)
  emit('drop', type, snap(p.x), snap(p.y))
}
</script>

<style scoped>
.handle {
  fill: #3d8bfd;
  stroke: #ffffff;
  stroke-width: 1;
  cursor: crosshair;
}
</style>
