<template>
  <div class="alarm-banner d-flex">
    <div class="rows flex-grow-1">
      <div v-for="a in shown" :key="a.id" class="row" :class="{ inactive: !a.active, 'alarm-acked': a.active && a.acked, clickable: a.view }" :title="a.view ? 'Visa ' + (a.station || 'vy') : ''" @click="go(a)">
        <AlarmIcon :alarm="a" />
        <span class="mono time">{{ fmt(a.raised) }}</span>
        <span class="station text-truncate">{{ a.station }}</span>
        <span class="source text-truncate">{{ a.source }}</span>
        <span class="message text-truncate">{{ a.message }}</span>
        <span class="state">{{ a.active ? 'Aktiv' : 'Återgått' }}</span>
        <v-btn v-if="!a.acked" size="x-small" variant="text" density="compact" @click.stop="live.ack([a.id])">Kvittera</v-btn>
      </div>
      <div v-if="!shown.length" class="row text-medium-emphasis">
        <v-icon icon="mdi-bell-outline" size="small" />
        <span>Inga larm</span>
      </div>
    </div>
    <div class="summary d-flex flex-column justify-center align-end px-3">
      <div class="text-caption">
        <span :class="{ 'text-error': live.unackedAlarms.length }">{{ live.unackedAlarms.length }} okvitterade</span>
        · {{ activeCount }} aktiva
      </div>
      <div>
        <v-btn size="x-small" variant="tonal" color="error" :disabled="!live.unackedAlarms.length" @click="live.ack()">Kvittera alla</v-btn>
        <v-btn size="x-small" variant="text" to="/alarms" class="ml-1">Larmlista</v-btn>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useLive } from '../stores/live'
import AlarmIcon from './AlarmIcon.vue'

const live = useLive()
const router = useRouter()

// go navigates to the view that shows the alarm object, if one is known.
function go(a) {
  if (a.view) router.push(`/views/${a.view}`)
}

// The three latest unacknowledged alarms or, when all are acknowledged, the
// three latest that are still active.
const shown = computed(() => {
  const list = live.unackedAlarms.length ? live.unackedAlarms : live.alarms.filter((a) => a.active)
  return list.slice(0, 3)
})
const activeCount = computed(() => live.alarms.filter((a) => a.active).length)
const fmt = (ts) => new Date(ts).toLocaleString('sv-SE')
</script>

<style scoped>
.alarm-banner { height: var(--alarm-h); border-bottom: 1px solid rgba(255, 255, 255, 0.08); background: #151b22; padding: 3px 0 3px 12px; }
.rows { min-width: 0; }
.row { display: grid; grid-template-columns: 20px 140px 130px minmax(110px, 240px) 1fr 66px 72px; gap: 8px; align-items: center; height: 24px; font-size: 13px; color: #ff6b6b; }
.row.inactive { color: #c9d1d9; }
.row.alarm-acked { color: var(--alarm-acked); }
.row.clickable { cursor: pointer; }
.row.clickable:hover { background: rgba(255, 255, 255, 0.05); }
.time { font-size: 12px; }
.station { font-size: 12px; opacity: 0.85; }
.state { font-size: 12px; opacity: 0.8; }
.summary { border-left: 1px solid rgba(255, 255, 255, 0.08); white-space: nowrap; gap: 4px; }
</style>
