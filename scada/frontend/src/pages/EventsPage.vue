<template>
  <v-container fluid class="fill-page">
    <div class="d-flex align-center mb-3 ga-3">
      <h2 class="text-h6">Händelser</h2>
      <v-spacer />
      <v-btn-toggle v-model="kind" density="compact" variant="outlined">
        <v-btn value="">Alla</v-btn>
        <v-btn value="command">Manövrar</v-btn>
        <v-btn value="state">Lägesändringar</v-btn>
        <v-btn value="comm">Kommunikation</v-btn>
        <v-btn value="system">System</v-btn>
      </v-btn-toggle>
    </div>
    <v-data-table :headers="headers" :items="items" density="compact" items-per-page="100">
      <template #item.ts="{ item }">
        <span class="mono">{{ new Date(item.ts).toLocaleString('sv-SE') }}</span>
      </template>
      <template #item.severity="{ item }">
        <v-icon v-if="item.severity === 'alarm'" color="error" icon="mdi-alert-circle" size="small" title="Larm" />
        <v-icon v-else color="info" icon="mdi-information-outline" size="small" title="Info" />
      </template>
      <template #item.kind="{ item }">{{ kinds[item.kind] || item.kind }}</template>
    </v-data-table>
  </v-container>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useLive } from '../stores/live'

const live = useLive()
const kind = ref('')
const kinds = { command: 'Manöver', state: 'Läge', comm: 'Kommunikation', system: 'System' }
const headers = [
  { title: '', key: 'severity', width: 32, sortable: false },
  { title: 'Tid', key: 'ts' },
  { title: 'Typ', key: 'kind' },
  { title: 'Objekt', key: 'source' },
  { title: 'Meddelande', key: 'message' },
  { title: 'Användare', key: 'user' },
]
const items = computed(() => (kind.value ? live.events.filter((e) => e.kind === kind.value) : live.events))

onMounted(() => (live.unseenAlarms = 0))
</script>
