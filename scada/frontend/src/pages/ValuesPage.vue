<template>
  <v-container fluid class="fill-page">
    <div class="d-flex align-center mb-3 ga-3">
      <h2 class="text-h6">Mätvärden</h2>
      <v-spacer />
      <v-text-field v-model="search" prepend-inner-icon="mdi-magnify" label="Sök" style="max-width: 300px" clearable />
    </div>
    <v-data-table
      :headers="headers"
      :items="rows"
      :search="search"
      density="compact"
      items-per-page="50"
      hover
      @click:row="(_, { item }) => (trendTag = item.id)"
    >
      <template #item.value="{ item }">
        <span class="mono" :class="item.q === 'good' ? '' : 'text-purple'">{{ item.value }}</span>
        <span class="text-medium-emphasis ms-1">{{ item.unit }}</span>
      </template>
      <template #item.q="{ item }">
        <v-chip size="x-small" :color="item.q === 'good' ? 'success' : 'purple'" variant="flat">{{ item.q === 'good' ? 'OK' : 'Osäker' }}</v-chip>
      </template>
      <template #item.actions="{ item }">
        <v-btn v-if="item.writable" size="x-small" variant="tonal" prepend-icon="mdi-pencil" @click.stop="openWrite(item)">Skriv</v-btn>
      </template>
    </v-data-table>

    <TrendDialog :tag-id="trendTag" @close="trendTag = null" />

    <v-dialog v-model="write.show" max-width="380">
      <v-card :title="`Skriv till ${write.tag && write.tag.id}`">
        <v-card-text>
          <v-text-field v-model.number="write.value" type="number" :label="`Värde ${write.tag && write.tag.unit}`" autofocus />
          <v-alert class="mt-3" type="warning" variant="tonal" density="compact">Värdet skrivs direkt till {{ write.tag && write.tag.device }} via Modbus TCP.</v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="write.show = false">Avbryt</v-btn>
          <v-btn color="warning" variant="flat" :loading="write.busy" @click="doWrite">Utför</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<script setup>
import { computed, inject, reactive, ref } from 'vue'
import TrendDialog from '../components/TrendDialog.vue'
import { useConfig } from '../stores/config'
import { useLive } from '../stores/live'
import { formatValue } from '../lib/symbols'
import { api } from '../lib/api'

const config = useConfig()
const live = useLive()
const notify = inject('notify')
const search = ref('')
const trendTag = ref(null)
const write = reactive({ show: false, tag: null, value: 0, busy: false })

const headers = [
  { title: 'Tagg', key: 'id' },
  { title: 'Namn', key: 'name' },
  { title: 'Värde', key: 'value', align: 'end', sortable: false },
  { title: 'Kvalitet', key: 'q' },
  { title: 'Senast ändrad', key: 'ts' },
  { title: 'Enhet', key: 'device' },
  { title: 'Adress', key: 'address' },
  { title: '', key: 'actions', sortable: false },
]

const rows = computed(() =>
  config.tags.map((t) => {
    const v = live.values[t.id]
    return {
      id: t.id,
      name: t.name,
      value: formatValue(v, t),
      unit: t.unit,
      q: v ? v.q : 'bad',
      ts: v ? new Date(v.ts).toLocaleTimeString('sv-SE') : '',
      device: t.device,
      address: `${t.area} ${t.address} · ${t.dataType}`,
      writable: t.writable,
    }
  }),
)

function openWrite(row) {
  const t = config.tagMap[row.id]
  const v = live.values[row.id]
  Object.assign(write, { show: true, tag: t, value: v ? v.v : 0 })
}

async function doWrite() {
  write.busy = true
  try {
    await api.command({ tag: write.tag.id, value: Number(write.value), source: write.tag.name, user: 'operatör' })
    notify(`Skrev ${write.value} till ${write.tag.id}`)
    write.show = false
  } catch (e) {
    notify(`Skrivning misslyckades: ${e.message}`, 'error')
  } finally {
    write.busy = false
  }
}
</script>
