<template>
  <div class="trend" ref="box">
    <svg :viewBox="`0 0 ${W} ${H}`" width="100%" @pointermove="hover" @pointerleave="hoverIdx = -1">
      <!-- recessive grid + axes -->
      <g v-for="t in yTicks" :key="'y' + t">
        <line :x1="M.l" :x2="W - M.r" :y1="y(t)" :y2="y(t)" stroke="currentColor" stroke-opacity="0.08" />
        <text :x="M.l - 6" :y="y(t)" text-anchor="end" dominant-baseline="central" class="axis">{{ fmt(t) }}</text>
      </g>
      <g v-for="t in xTicks" :key="'x' + t">
        <text :x="x(t)" :y="H - M.b + 16" text-anchor="middle" class="axis">{{ time(t) }}</text>
      </g>
      <line :x1="M.l" :x2="W - M.r" :y1="H - M.b" :y2="H - M.b" stroke="currentColor" stroke-opacity="0.25" />

      <polyline v-if="points.length > 1" :points="path" fill="none" stroke="#3d8bfd" stroke-width="2" stroke-linejoin="round" />
      <text v-else :x="W / 2" :y="H / 2" text-anchor="middle" class="axis">Ingen historik ännu</text>

      <!-- crosshair + tooltip -->
      <g v-if="hoverIdx >= 0 && points[hoverIdx]">
        <line :x1="x(points[hoverIdx].t)" :x2="x(points[hoverIdx].t)" :y1="M.t" :y2="H - M.b" stroke="currentColor" stroke-opacity="0.35" />
        <circle :cx="x(points[hoverIdx].t)" :cy="y(points[hoverIdx].v)" r="4" fill="#3d8bfd" stroke="rgb(var(--v-theme-surface))" stroke-width="2" />
      </g>
    </svg>
    <div v-if="hoverIdx >= 0 && points[hoverIdx]" class="tip" :style="tipStyle">
      <div class="text-caption text-medium-emphasis">{{ time(points[hoverIdx].t, true) }}</div>
      <div class="mono">{{ fmt(points[hoverIdx].v, decimals) }} {{ unit }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'

const props = defineProps({
  samples: { type: Array, default: () => [] }, // [{t, v}]
  decimals: { type: Number, default: 2 },
  unit: { type: String, default: '' },
})

const W = 720
const H = 280
const M = { l: 64, r: 16, t: 12, b: 28 }
const box = ref(null)
const hoverIdx = ref(-1)

const points = computed(() => props.samples)
const tRange = computed(() => {
  const p = points.value
  if (!p.length) return [Date.now() - 60000, Date.now()]
  return [p[0].t, Math.max(p[p.length - 1].t, p[0].t + 1000)]
})

function niceStep(span, count) {
  const raw = span / count
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const n = raw / mag
  return (n < 1.5 ? 1 : n < 3 ? 2 : n < 7 ? 5 : 10) * mag
}

const yDomain = computed(() => {
  const vs = points.value.map((p) => p.v)
  let lo = vs.length ? Math.min(...vs) : 0
  let hi = vs.length ? Math.max(...vs) : 1
  if (hi - lo < 1e-9) {
    const pad = Math.abs(hi) * 0.01 || 1
    lo -= pad
    hi += pad
  }
  const step = niceStep(hi - lo, 5)
  return [Math.floor(lo / step) * step, Math.ceil(hi / step) * step, step]
})
const yTicks = computed(() => {
  const [lo, hi, step] = yDomain.value
  const out = []
  for (let v = lo; v <= hi + step / 2; v += step) out.push(+v.toPrecision(12))
  return out
})
const xTicks = computed(() => {
  const [a, b] = tRange.value
  const steps = [5e3, 10e3, 15e3, 30e3, 60e3, 120e3, 300e3, 600e3, 900e3]
  const step = steps.find((s) => (b - a) / s <= 6) || 1800e3
  const out = []
  for (let t = Math.ceil(a / step) * step; t <= b; t += step) out.push(t)
  return out
})

const x = (t) => M.l + ((t - tRange.value[0]) / (tRange.value[1] - tRange.value[0])) * (W - M.l - M.r)
const y = (v) => {
  const [lo, hi] = yDomain.value
  return H - M.b - ((v - lo) / (hi - lo)) * (H - M.t - M.b)
}
const path = computed(() => points.value.map((p) => `${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(' '))

const fmt = (v, d) => {
  const dec = d ?? Math.max(0, -Math.floor(Math.log10(yDomain.value[2])))
  return v.toLocaleString('sv-SE', { minimumFractionDigits: dec, maximumFractionDigits: dec })
}
const time = (t, sec) => new Date(t).toLocaleTimeString('sv-SE', sec ? undefined : { hour: '2-digit', minute: '2-digit', second: '2-digit' })

function hover(evt) {
  const p = points.value
  if (!p.length) return
  const rect = evt.currentTarget.getBoundingClientRect()
  const sx = ((evt.clientX - rect.left) / rect.width) * W
  const t = tRange.value[0] + ((sx - M.l) / (W - M.l - M.r)) * (tRange.value[1] - tRange.value[0])
  let lo = 0
  let hi = p.length - 1
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1
    if (p[mid].t < t) lo = mid
    else hi = mid
  }
  hoverIdx.value = Math.abs(p[lo].t - t) < Math.abs(p[hi].t - t) ? lo : hi
}

const tipStyle = computed(() => {
  const p = points.value[hoverIdx.value]
  const left = (x(p.t) / W) * 100
  return { left: left > 70 ? undefined : `calc(${left}% + 12px)`, right: left > 70 ? `calc(${100 - left}% + 12px)` : undefined, top: '8px' }
})
</script>

<style scoped>
.trend { position: relative; color: rgb(var(--v-theme-on-surface)); }
.axis { font-size: 11px; fill: currentColor; fill-opacity: 0.6; }
.tip {
  position: absolute;
  pointer-events: none;
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 6px;
  padding: 4px 8px;
}
</style>
