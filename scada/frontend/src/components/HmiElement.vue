<template>
  <g :class="{ 'hmi-clickable': clickable }" @click="onClick">
    <!-- invisible hit area so thin shapes are easy to click/drag -->
    <rect v-if="!def.line" class="hmi-hover" :x="-2" :y="-2" :width="el.w + 4" :height="el.h + 4" fill="transparent" stroke="none" />

    <!-- ===== static graphics ===== -->
    <template v-if="def.line">
      <line x1="0" y1="0" :x2="el.w" :y2="el.h" stroke="transparent" :stroke-width="Math.max(12, p.strokeWidth + 8)" />
      <line x1="0" y1="0" :x2="el.w" :y2="el.h" :stroke="dyn || p.stroke" :stroke-width="p.strokeWidth" :stroke-dasharray="p.dash || null" stroke-linecap="round" />
    </template>

    <rect v-else-if="el.type === 'rect'" :width="el.w" :height="el.h" :rx="p.radius" :fill="p.fill" :stroke="dyn || p.stroke" :stroke-width="p.strokeWidth" :stroke-dasharray="p.dash || null" />

    <ellipse v-else-if="el.type === 'ellipse'" :cx="el.w / 2" :cy="el.h / 2" :rx="el.w / 2" :ry="el.h / 2" :fill="p.fill" :stroke="dyn || p.stroke" :stroke-width="p.strokeWidth" />

    <text v-else-if="el.type === 'text'" :x="textX" :y="el.h / 2" dominant-baseline="central" :text-anchor="p.align" :font-size="p.fontSize" :font-weight="p.bold ? 700 : 400" :fill="dyn || p.color">{{ p.text }}</text>

    <template v-else-if="el.type === 'image'">
      <image v-if="p.src" :href="p.src" :width="el.w" :height="el.h" :preserveAspectRatio="p.fit" :opacity="p.opacity" />
      <template v-else>
        <rect :width="el.w" :height="el.h" fill="none" stroke="#5c6b7a" stroke-dasharray="4 4" />
        <text :x="el.w / 2" :y="el.h / 2" text-anchor="middle" dominant-baseline="central" fill="#5c6b7a" font-size="12">Ingen bild vald</text>
      </template>
    </template>

    <g v-else-if="el.type === 'transformer'" fill="none" :stroke="dyn || p.stroke" :stroke-width="p.strokeWidth">
      <circle :cx="el.w / 2" :cy="el.h * 0.36" :r="Math.min(el.w / 2, el.h * 0.36) - 1" />
      <circle :cx="el.w / 2" :cy="el.h * 0.64" :r="Math.min(el.w / 2, el.h * 0.36) - 1" />
    </g>

    <g v-else-if="el.type === 'ground'" :stroke="p.stroke" :stroke-width="p.strokeWidth">
      <line :x1="el.w / 2" y1="0" :x2="el.w / 2" :y2="el.h * 0.4" />
      <line x1="0" :y1="el.h * 0.4" :x2="el.w" :y2="el.h * 0.4" />
      <line :x1="el.w * 0.2" :y1="el.h * 0.65" :x2="el.w * 0.8" :y2="el.h * 0.65" />
      <line :x1="el.w * 0.4" :y1="el.h * 0.9" :x2="el.w * 0.6" :y2="el.h * 0.9" />
    </g>

    <!-- ===== dynamic symbols ===== -->
    <g v-else-if="el.type === 'breaker'">
      <rect
        x="2" y="2" :width="el.w - 4" :height="el.h - 4" rx="2"
        :fill="sw === 'closed' ? swColor : '#00000000'"
        :stroke="swColor" stroke-width="3"
      />
      <template v-if="sw === 'intermediate'">
        <line x1="4" y1="4" :x2="el.w - 4" :y2="el.h - 4" :stroke="swColor" stroke-width="3" />
        <line :x1="el.w - 4" y1="4" x2="4" :y2="el.h - 4" :stroke="swColor" stroke-width="3" />
      </template>
      <text v-if="sw === 'bad' || sw === 'unknown'" :x="el.w / 2" :y="el.h / 2" text-anchor="middle" dominant-baseline="central" :fill="swColor" font-size="18" font-weight="700">?</text>
      <text v-if="p.showLabel" :x="el.w + 6" :y="el.h / 2" dominant-baseline="central" fill="#c9d1d9" font-size="13">{{ p.label }}</text>
    </g>

    <g v-else-if="el.type === 'disconnector'" :stroke="swColor" stroke-width="3" stroke-linecap="round">
      <line :x1="el.w / 2" y1="0" :x2="el.w / 2" :y2="el.h * 0.25" />
      <line :x1="el.w * 0.25" :y1="el.h * 0.25" :x2="el.w * 0.75" :y2="el.h * 0.25" />
      <line :x1="el.w / 2" :y1="el.h * 0.75" :x2="el.w / 2" :y2="el.h" />
      <line v-if="sw === 'closed'" :x1="el.w / 2" :y1="el.h * 0.75" :x2="el.w / 2" :y2="el.h * 0.25" />
      <line v-else-if="sw === 'open'" :x1="el.w / 2" :y1="el.h * 0.75" :x2="el.w * 0.05" :y2="el.h * 0.32" />
      <line v-else :x1="el.w / 2" :y1="el.h * 0.75" :x2="el.w * 0.3" :y2="el.h * 0.28" stroke-dasharray="3 4" />
      <circle :cx="el.w / 2" :cy="el.h * 0.75" r="3" :fill="swColor" />
      <text v-if="sw === 'bad' || sw === 'unknown'" :x="el.w * 0.85" :y="el.h / 2" text-anchor="middle" dominant-baseline="central" :fill="swColor" stroke="none" font-size="16" font-weight="700">?</text>
      <text v-if="p.showLabel" :x="el.w + 4" :y="el.h / 2" dominant-baseline="central" fill="#c9d1d9" stroke="none" font-size="13">{{ p.label }}</text>
    </g>

    <g v-else-if="el.type === 'toggle'">
      <rect :width="el.w" :height="el.h" rx="3" fill="#0d1117" stroke="#30363d" />
      <text x="8" :y="el.h / 2" dominant-baseline="central" font-size="13" fill="#8b949e">{{ p.label }}</text>
      <rect :x="el.w - 58" y="4" width="52" :height="el.h - 8" :rx="(el.h - 8) / 2" :fill="toggleFill" />
      <text :x="el.w - 32" :y="el.h / 2" text-anchor="middle" dominant-baseline="central" font-size="12" font-weight="700" fill="#ffffff">{{ toggleText }}</text>
    </g>

    <g v-else-if="el.type === 'pump'">
      <circle :cx="el.w / 2" :cy="el.h / 2" :r="Math.min(el.w, el.h) / 2 - 2" :fill="sw === 'closed' ? swColor : '#161b22'" :stroke="swColor" stroke-width="2.5" />
      <polygon :points="`${el.w * 0.36},${el.h * 0.28} ${el.w * 0.36},${el.h * 0.72} ${el.w * 0.76},${el.h / 2}`" :fill="sw === 'closed' ? '#0d1117' : swColor" />
      <text v-if="sw === 'bad' || sw === 'unknown'" :x="el.w / 2" :y="-6" text-anchor="middle" :fill="swColor" font-size="13" font-weight="700">?</text>
      <text v-if="p.showLabel" :x="el.w / 2" :y="el.h + 13" text-anchor="middle" fill="#c9d1d9" font-size="12">{{ p.label }}</text>
    </g>

    <g v-else-if="el.type === 'valve'">
      <polygon :points="`2,2 ${el.w / 2},${el.h / 2} 2,${el.h - 2}`" :fill="sw === 'closed' ? swColor : '#161b22'" :stroke="swColor" stroke-width="2" stroke-linejoin="round" />
      <polygon :points="`${el.w - 2},2 ${el.w / 2},${el.h / 2} ${el.w - 2},${el.h - 2}`" :fill="sw === 'closed' ? swColor : '#161b22'" :stroke="swColor" stroke-width="2" stroke-linejoin="round" />
      <text v-if="sw === 'bad' || sw === 'unknown'" :x="el.w / 2" :y="-6" text-anchor="middle" :fill="swColor" font-size="13" font-weight="700">?</text>
      <text v-if="p.showLabel" :x="el.w / 2" :y="el.h + 13" text-anchor="middle" fill="#c9d1d9" font-size="12">{{ p.label }}</text>
    </g>

    <g v-else-if="el.type === 'measurement'">
      <rect :width="el.w" :height="el.h" rx="3" :fill="p.background" :stroke="inAlarm ? p.alarmColor : p.border" :stroke-width="inAlarm ? 2 : 1" />
      <text v-if="p.label" x="8" :y="el.h / 2" dominant-baseline="central" :font-size="p.fontSize * 0.8" fill="#8b949e">{{ p.label }}</text>
      <text :x="el.w - 8" :y="el.h / 2" text-anchor="end" dominant-baseline="central" :font-size="p.fontSize" class="mono" :fill="valueColor">
        {{ formatted }}<tspan dx="4" :font-size="p.fontSize * 0.75" fill="#8b949e">{{ unit }}</tspan>
      </text>
    </g>

    <g v-else-if="el.type === 'indicator'">
      <circle :cx="el.w / 2" :cy="el.h / 2" :r="Math.min(el.w, el.h) / 2 - 1" :fill="indicatorColor" stroke="#0d1117" stroke-width="1" :class="{ 'hmi-blink': p.blink && indicatorOn }" />
      <text v-if="p.label" :x="el.w + 6" :y="el.h / 2" dominant-baseline="central" fill="#c9d1d9" font-size="13">{{ p.label }}</text>
    </g>

    <g v-else-if="el.type === 'bar'">
      <rect :width="el.w" :height="el.h" :fill="p.background" stroke="#30363d" />
      <rect v-if="el.w > el.h" x="1" y="1" :width="barFrac * (el.w - 2)" :height="el.h - 2" :fill="good ? p.color : '#b04dff'" />
      <rect v-else x="1" :y="el.h - 1 - barFrac * (el.h - 2)" :width="el.w - 2" :height="barFrac * (el.h - 2)" :fill="good ? p.color : '#b04dff'" />
      <text v-if="p.showValue" :x="el.w / 2" :y="el.h + 14" text-anchor="middle" font-size="12" class="mono" fill="#c9d1d9">{{ formatted }}</text>
    </g>

    <g v-else-if="el.type === 'button' || el.type === 'link'">
      <rect :width="el.w" :height="el.h" rx="4" :fill="p.color" />
      <text :x="el.w / 2" :y="el.h / 2" text-anchor="middle" dominant-baseline="central" :fill="p.textColor" font-size="14" font-weight="500">{{ p.text }}</text>
    </g>
  </g>
</template>

<script setup>
import { computed } from 'vue'
import { useLive } from '../stores/live'
import { useConfig } from '../stores/config'
import { catalog, dynColor, formatValue, num, switchState } from '../lib/symbols'

const props = defineProps({
  el: { type: Object, required: true },
  runtime: { type: Boolean, default: false },
})
const emit = defineEmits(['activate'])

const live = useLive()
const config = useConfig()

const def = computed(() => catalog[props.el.type] || catalog.rect)
const p = computed(() => props.el.props || {})

const valueOf = (tag) => (tag ? live.values[tag] : undefined)

const dyn = computed(() => dynColor(p.value, valueOf(p.value.dynTag)))
const textX = computed(() => ({ start: 0, middle: props.el.w / 2, end: props.el.w }[p.value.align] ?? 0))

// switching devices
const sw = computed(() => switchState(p.value, valueOf(p.value.statusTag)))
const swColor = computed(() => {
  switch (sw.value) {
    case 'closed': return p.value.colorClosed
    case 'open': return p.value.colorOpen
    case 'intermediate': return '#ffb300'
    case 'bad': return '#b04dff'
    default: return '#5c6b7a'
  }
})

const toggleFill = computed(() => (sw.value === 'closed' ? p.value.colorClosed : sw.value === 'open' ? p.value.colorOpen : sw.value === 'bad' ? '#b04dff' : '#ffb300'))
const toggleText = computed(() => ({ closed: p.value.onText, open: p.value.offText, bad: '???', unknown: '–' }[sw.value] ?? '?'))

// analog values
const value = computed(() => valueOf(p.value.tag))
const tagDef = computed(() => config.tagMap[p.value.tag])
const good = computed(() => value.value && value.value.q === 'good')
const formatted = computed(() => formatValue(value.value, tagDef.value, p.value.decimals))
const unit = computed(() => p.value.unit || (tagDef.value && tagDef.value.unit) || '')
const inAlarm = computed(() => {
  if (!good.value) return false
  const v = value.value.v
  const lo = num(p.value.lowLimit, -Infinity)
  const hi = num(p.value.highLimit, Infinity)
  return v < lo || v > hi
})
const valueColor = computed(() => (!good.value ? '#b04dff' : inAlarm.value ? p.value.alarmColor : p.value.color))

const indicatorOn = computed(() => good.value && value.value.v === num(p.value.onValue, 1))
const indicatorColor = computed(() => (!good.value ? '#b04dff' : indicatorOn.value ? p.value.onColor : p.value.offColor))

// fill fraction 0..1; horizontal when the bar is wider than tall
const barFrac = computed(() => {
  if (!good.value) return 1
  const min = num(p.value.min, 0)
  const max = num(p.value.max, 100)
  return Math.max(0, Math.min(1, (value.value.v - min) / (max - min || 1)))
})

const clickable = computed(() => {
  if (!props.runtime || !def.value.interactive) return false
  const x = p.value
  return !!(x.statusTag || x.controlTag || x.tag || x.view)
})

function onClick(e) {
  if (clickable.value) {
    e.stopPropagation()
    emit('activate', props.el)
  }
}
</script>

<style>
@keyframes hmi-blink { 50% { opacity: 0.2; } }
.hmi-blink { animation: hmi-blink 1s step-start infinite; }
</style>
