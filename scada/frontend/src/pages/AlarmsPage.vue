<template>
  <v-container fluid class="fill-page">
    <div class="d-flex align-center mb-3 ga-3">
      <h2 class="text-h6">Larm</h2>
      <v-spacer />
      <v-btn color="error" variant="tonal" prepend-icon="mdi-check-all" :disabled="!live.unackedAlarms.length" @click="live.ack()">Kvittera alla</v-btn>
    </div>
    <v-data-table :headers="headers" :items="live.alarms" density="compact" items-per-page="100" no-data-text="Inga larm" hover @click:row="onRow">
      <template #item.icon="{ item }"><AlarmIcon :alarm="item" /></template>
      <template #item.raised="{ item }"><span class="mono">{{ fmt(item.raised) }}</span></template>
      <template #item.kind="{ item }">{{ kinds[item.kind] || item.kind }}</template>
      <template #item.station="{ item }">
        <RouterLink v-if="item.view" :to="`/views/${item.view}`" class="station-link" @click.stop>{{ item.station }}</RouterLink>
        <span v-else>{{ item.station }}</span>
      </template>
      <template #item.state="{ item }">
        <span v-if="item.active" :class="item.acked ? 'alarm-acked' : 'text-error'">Aktiv</span>
        <span v-else>Återgått <span class="mono text-caption">{{ fmt(item.cleared) }}</span></span>
      </template>
      <template #item.ack="{ item }">
        <span v-if="item.acked" class="text-caption"><span class="mono">{{ fmt(item.ackedAt) }}</span> {{ item.ackedBy }}</span>
        <v-btn v-else size="small" variant="text" color="error" @click="live.ack([item.id])">Kvittera</v-btn>
      </template>
    </v-data-table>
  </v-container>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { useLive } from '../stores/live'
import AlarmIcon from '../components/AlarmIcon.vue'

const live = useLive()
const router = useRouter()
const kinds = { state: 'Process', comm: 'Kommunikation' }
const headers = [
  { title: '', key: 'icon', width: 32, sortable: false },
  { title: 'Tid', key: 'raised' },
  { title: 'Typ', key: 'kind' },
  { title: 'Station', key: 'station' },
  { title: 'Objekt', key: 'source' },
  { title: 'Meddelande', key: 'message' },
  { title: 'Tillstånd', key: 'state', sortable: false },
  { title: 'Kvitterat', key: 'ack', sortable: false },
]
const fmt = (ts) => (ts ? new Date(ts).toLocaleString('sv-SE') : '')

// onRow navigates to the view that shows the alarm object.
function onRow(_event, { item }) {
  if (item.view) router.push(`/views/${item.view}`)
}
</script>

<style scoped>
.station-link { color: #4a90d9; text-decoration: none; }
.station-link:hover { text-decoration: underline; }
</style>
