<template>
  <v-container fluid class="fill-page">
    <v-tabs v-model="tab" class="mb-3">
      <v-tab value="devices" prepend-icon="mdi-server-network">Enheter (Modbus TCP)</v-tab>
      <v-tab value="tags" prepend-icon="mdi-tag-multiple">Taggar</v-tab>
    </v-tabs>

    <!-- devices -->
    <div v-if="tab === 'devices'">
      <div class="d-flex mb-2">
        <v-spacer />
        <v-btn color="primary" prepend-icon="mdi-plus" @click="editDevice(null)">Ny enhet</v-btn>
      </div>
      <v-data-table :headers="deviceHeaders" :items="config.devices" density="compact" hover @click:row="(_, { item }) => editDevice(item)">
        <template #item.status="{ item }">
          <v-chip v-if="!item.enabled" size="x-small">Avstängd</v-chip>
          <v-chip v-else-if="live.devices[item.id]" size="x-small" variant="flat" :color="live.devices[item.id].connected ? 'success' : 'error'" :title="live.devices[item.id].error">
            {{ live.devices[item.id].connected ? 'Ansluten' : 'Fel' }}
          </v-chip>
        </template>
        <template #item.addr="{ item }"><span class="mono">{{ item.host }}:{{ item.port }} / {{ item.unitId }}</span></template>
      </v-data-table>
    </div>

    <!-- tags -->
    <div v-else>
      <div class="d-flex mb-2 ga-3">
        <v-text-field v-model="search" prepend-inner-icon="mdi-magnify" label="Sök" style="max-width: 300px" clearable />
        <v-spacer />
        <v-btn color="primary" prepend-icon="mdi-plus" :disabled="!config.devices.length" @click="editTag(null)">Ny tagg</v-btn>
      </div>
      <v-data-table :headers="tagHeaders" :items="config.tags" :search="search" density="compact" items-per-page="50" hover @click:row="(_, { item }) => editTag(item)">
        <template #item.addr="{ item }"><span class="mono">{{ item.area }} {{ item.address }}</span></template>
        <template #item.writable="{ item }"><v-icon v-if="item.writable" icon="mdi-pencil" size="small" /></template>
        <template #item.logEvents="{ item }"><v-icon v-if="item.logEvents" icon="mdi-format-list-bulleted" size="small" /></template>
      </v-data-table>
    </div>

    <!-- device dialog -->
    <v-dialog v-model="dev.show" max-width="520">
      <v-card :title="dev.oldId ? `Enhet ${dev.oldId}` : 'Ny enhet'">
        <v-card-text>
          <v-row dense>
            <v-col cols="6"><v-text-field v-model="dev.d.id" label="Id" /></v-col>
            <v-col cols="6"><v-text-field v-model="dev.d.name" label="Namn" /></v-col>
            <v-col cols="8"><v-text-field v-model="dev.d.host" label="IP-adress / värdnamn" /></v-col>
            <v-col cols="4"><v-text-field v-model.number="dev.d.port" label="Port" type="number" /></v-col>
            <v-col cols="4"><v-text-field v-model.number="dev.d.unitId" label="Unit id" type="number" /></v-col>
            <v-col cols="4"><v-text-field v-model.number="dev.d.pollMs" label="Pollintervall (ms)" type="number" /></v-col>
            <v-col cols="4"><v-text-field v-model.number="dev.d.timeoutMs" label="Timeout (ms)" type="number" /></v-col>
            <v-col cols="12"><v-switch v-model="dev.d.enabled" label="Aktiv" /></v-col>
          </v-row>
          <v-alert v-if="dev.error" type="error" variant="tonal" density="compact" class="mt-2">{{ dev.error }}</v-alert>
        </v-card-text>
        <v-card-actions>
          <v-btn v-if="dev.oldId" color="error" variant="text" @click="deleteDevice">Ta bort</v-btn>
          <v-spacer />
          <v-btn @click="dev.show = false">Avbryt</v-btn>
          <v-btn color="primary" variant="flat" @click="saveDevice">Spara</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- tag dialog -->
    <v-dialog v-model="tg.show" max-width="640">
      <v-card :title="tg.oldId ? `Tagg ${tg.oldId}` : 'Ny tagg'">
        <v-card-text>
          <v-row dense>
            <v-col cols="5"><v-text-field v-model="tg.t.id" label="Id" /></v-col>
            <v-col cols="7"><v-text-field v-model="tg.t.name" label="Namn" /></v-col>
            <v-col cols="12"><v-text-field v-model="tg.t.description" label="Beskrivning" /></v-col>
            <v-col cols="4"><v-select v-model="tg.t.device" :items="config.devices" item-title="id" item-value="id" label="Enhet" /></v-col>
            <v-col cols="4"><v-select v-model="tg.t.area" :items="areas" label="Område" /></v-col>
            <v-col cols="4"><v-text-field v-model.number="tg.t.address" label="Adress (0-baserad)" type="number" /></v-col>
            <template v-if="!isBitArea">
              <v-col cols="4"><v-select v-model="tg.t.dataType" :items="dataTypes" label="Datatyp" /></v-col>
              <v-col cols="4"><v-switch v-model="tg.t.wordSwap" label="Ordbyte (CDAB)" /></v-col>
              <v-col cols="4"><v-switch v-model="tg.t.byteSwap" label="Bytebyte" /></v-col>
              <v-col v-if="tg.t.dataType === 'bool'" cols="4"><v-text-field v-model.number="tg.t.bit" label="Bit (-1 = hela registret)" type="number" /></v-col>
              <template v-else>
                <v-col cols="4"><v-text-field v-model.number="tg.t.scale" label="Skala" type="number" /></v-col>
                <v-col cols="4"><v-text-field v-model.number="tg.t.offset" label="Offset" type="number" /></v-col>
              </template>
            </template>
            <v-col cols="4"><v-text-field v-model="tg.t.unit" label="Enhet (t.ex. kV)" /></v-col>
            <v-col cols="4"><v-text-field v-model.number="tg.t.decimals" label="Decimaler" type="number" /></v-col>
            <v-col cols="6"><v-switch v-model="tg.t.writable" label="Skrivbar (manöver)" :disabled="tg.t.area === 'input' || tg.t.area === 'discrete'" /></v-col>
            <v-col cols="6"><v-switch v-model="tg.t.logEvents" label="Logga ändringar som händelser" /></v-col>
            <v-col cols="12">
              <v-text-field v-model="tg.stateText" label="Lägestexter (t.ex. 0=Mellanläge, 1=Öppen, 2=Sluten, 3=Fel)" />
            </v-col>
          </v-row>
          <div class="text-caption text-medium-emphasis mt-2">
            Värde = råvärde × skala + offset. 32/64-bitarsvärden läses som big-endian (ABCD); använd ord-/bytebyte för andra ordningar.
          </div>
          <v-alert v-if="tg.error" type="error" variant="tonal" density="compact" class="mt-2">{{ tg.error }}</v-alert>
        </v-card-text>
        <v-card-actions>
          <v-btn v-if="tg.oldId" color="error" variant="text" @click="deleteTag">Ta bort</v-btn>
          <v-spacer />
          <v-btn @click="tg.show = false">Avbryt</v-btn>
          <v-btn color="primary" variant="flat" @click="saveTag">Spara</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<script setup>
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { useConfig } from '../stores/config'
import { useLive } from '../stores/live'
import { api } from '../lib/api'

const config = useConfig()
const live = useLive()
const notify = inject('notify')
const tab = ref('devices')
const search = ref('')

const areas = [
  { title: 'Coil (0x, läs/skriv bit)', value: 'coil' },
  { title: 'Discrete input (1x, läs bit)', value: 'discrete' },
  { title: 'Input register (3x)', value: 'input' },
  { title: 'Holding register (4x)', value: 'holding' },
]
const dataTypes = ['bool', 'uint16', 'int16', 'uint32', 'int32', 'float32', 'float64']

const deviceHeaders = [
  { title: 'Id', key: 'id' },
  { title: 'Namn', key: 'name' },
  { title: 'Adress / unit', key: 'addr', sortable: false },
  { title: 'Poll (ms)', key: 'pollMs' },
  { title: 'Status', key: 'status', sortable: false },
]
const tagHeaders = [
  { title: 'Id', key: 'id' },
  { title: 'Namn', key: 'name' },
  { title: 'Enhet', key: 'device' },
  { title: 'Adress', key: 'addr', sortable: false },
  { title: 'Datatyp', key: 'dataType' },
  { title: 'Enhet', key: 'unit' },
  { title: 'Skrivbar', key: 'writable' },
  { title: 'Händelser', key: 'logEvents' },
]

const dev = reactive({ show: false, oldId: '', d: {}, error: '' })
const tg = reactive({ show: false, oldId: '', t: {}, stateText: '', error: '' })
const isBitArea = computed(() => tg.t.area === 'coil' || tg.t.area === 'discrete')

function editDevice(d) {
  Object.assign(dev, {
    show: true,
    error: '',
    oldId: d ? d.id : '',
    d: d ? { ...d } : { id: '', name: '', host: '127.0.0.1', port: 502, unitId: 1, pollMs: 1000, timeoutMs: 2000, enabled: true },
  })
}

async function saveDevice() {
  try {
    await api.saveDevice(dev.oldId, dev.d)
    await config.load()
    dev.show = false
    notify('Enhet sparad')
  } catch (e) {
    dev.error = e.message
  }
}

async function deleteDevice() {
  if (!confirm(`Ta bort enheten ${dev.oldId}?`)) return
  try {
    await api.deleteDevice(dev.oldId)
    await config.load()
    dev.show = false
  } catch (e) {
    dev.error = e.message
  }
}

function editTag(t) {
  const base = { id: '', name: '', description: '', device: config.devices[0]?.id, area: 'holding', address: 0, dataType: 'float32', bit: -1, wordSwap: false, byteSwap: false, scale: 1, offset: 0, unit: '', decimals: 2, writable: false, logEvents: false }
  const tag = t ? { ...base, ...JSON.parse(JSON.stringify(t)) } : base
  Object.assign(tg, {
    show: true,
    error: '',
    oldId: t ? t.id : '',
    t: tag,
    stateText: Object.entries(tag.stateText || {}).map(([k, v]) => `${k}=${v}`).join(', '),
  })
}

function parseStateText(s) {
  const out = {}
  for (const part of s.split(',')) {
    const [k, ...v] = part.split('=')
    if (k && k.trim() && v.length) out[k.trim()] = v.join('=').trim()
  }
  return out
}

async function saveTag() {
  try {
    const t = { ...tg.t, stateText: parseStateText(tg.stateText) }
    if (isBitArea.value) t.dataType = 'bool'
    await api.saveTag(tg.oldId, t)
    await config.load()
    tg.show = false
    notify('Tagg sparad')
  } catch (e) {
    tg.error = e.message
  }
}

async function deleteTag() {
  if (!confirm(`Ta bort taggen ${tg.oldId}?`)) return
  try {
    await api.deleteTag(tg.oldId)
    await config.load()
    tg.show = false
  } catch (e) {
    tg.error = e.message
  }
}

onMounted(() => config.load())
</script>
