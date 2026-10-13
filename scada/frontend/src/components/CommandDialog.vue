<template>
  <v-dialog :model-value="!!el" max-width="440" persistent>
    <v-card v-if="el">
      <v-card-title class="d-flex align-center">
        <v-icon :icon="isSwitch ? 'mdi-electric-switch' : 'mdi-gesture-tap-button'" class="mr-2" />
        {{ title }}
      </v-card-title>
      <v-card-subtitle v-if="objectName && objectName !== title">{{ objectName }}</v-card-subtitle>
      <v-card-text>
        <template v-if="isSwitch">
          <div class="d-flex justify-space-between mb-1">
            <span class="text-medium-emphasis">Läge</span>
            <v-chip size="small" :color="stateColor" variant="flat">{{ stateLabel }}</v-chip>
          </div>
          <div class="d-flex justify-space-between text-caption text-medium-emphasis mb-3">
            <span>Indikering: {{ el.props.statusTag || '–' }}</span>
            <span>Manöver: {{ el.props.controlTag || '–' }}</span>
          </div>
        </template>

        <v-alert v-if="!controlTag" type="info" variant="tonal" density="compact">
          Symbolen saknar manövertagg – endast indikering.
        </v-alert>
        <v-alert v-else-if="controlTag && !controlTag.writable" type="warning" variant="tonal" density="compact">
          Taggen {{ controlTag.id }} är inte skrivbar.
        </v-alert>

        <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3" closable @click:close="error = ''">
          Manöver misslyckades: {{ error }}
        </v-alert>

        <!-- step 1: select operation -->
        <template v-else-if="phase === 'select'">
          <div v-if="isSwitch" class="d-flex ga-3">
            <v-btn class="flex-grow-1" color="success" variant="tonal" size="large" :disabled="state === 'open'" prepend-icon="mdi-power-off" @click="choose('open')">
              {{ opText.open }}
            </v-btn>
            <v-btn class="flex-grow-1" color="error" variant="tonal" size="large" :disabled="state === 'closed'" prepend-icon="mdi-power" @click="choose('close')">
              {{ opText.close }}
            </v-btn>
          </div>
          <template v-else>
            <v-text-field v-if="el.props.askValue" v-model.number="setpoint" type="number" :label="`Nytt värde ${controlTag.unit || ''}`" autofocus class="mb-3" />
            <v-btn block color="primary" variant="tonal" size="large" @click="choose('write')">Välj</v-btn>
          </template>
        </template>

        <!-- step 2: confirm (select-before-operate) -->
        <template v-else-if="phase === 'confirm'">
          <v-alert type="warning" variant="tonal" class="mb-3">
            Bekräfta manöver: <strong>{{ pending.text }}</strong> på <strong>{{ title }}</strong>
            <div class="text-caption">Skriver {{ pending.value }} till {{ controlTag.id }}</div>
          </v-alert>
          <div class="d-flex ga-3">
            <v-btn class="flex-grow-1" variant="outlined" @click="phase = 'select'">Avbryt</v-btn>
            <v-btn class="flex-grow-1" color="warning" variant="flat" :loading="busy" @click="execute">Utför</v-btn>
          </div>
        </template>

        <!-- step 3: wait for position change -->
        <template v-else-if="phase === 'wait'">
          <div class="d-flex align-center ga-3">
            <v-progress-circular v-if="waiting" indeterminate size="22" />
            <v-icon v-else-if="success" color="success" icon="mdi-check-circle" />
            <v-icon v-else color="warning" icon="mdi-alert" />
            <span>{{ waitText }}</span>
          </div>
        </template>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="close">Stäng</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { computed, inject, onUnmounted, ref, watch } from 'vue'
import { useLive } from '../stores/live'
import { useConfig } from '../stores/config'
import { api } from '../lib/api'
import { num, stateText, switchState } from '../lib/symbols'

const props = defineProps({ el: { type: Object, default: null } })
const emit = defineEmits(['close'])
const notify = inject('notify')
const live = useLive()
const config = useConfig()

const phase = ref('select')
const pending = ref(null)
const busy = ref(false)
const setpoint = ref(0)
const waiting = ref(false)
const success = ref(false)
const waitText = ref('')
const error = ref('')
let timeout = null

const isSwitch = computed(() => props.el && ['breaker', 'disconnector', 'toggle'].includes(props.el.type))
// button texts for the two operations
const opText = computed(() => {
  const el = props.el
  if (!el) return {}
  if (el.type === 'breaker') return { open: 'Frånslag', close: 'Tillslag' }
  if (el.type === 'toggle') return { open: el.props.offText || 'AV', close: el.props.onText || 'PÅ' }
  return { open: 'Öppna', close: 'Slut' }
})
const stateLabel = computed(() => {
  if (props.el && props.el.type === 'toggle' && (state.value === 'open' || state.value === 'closed')) return state.value === 'open' ? opText.value.open : opText.value.close
  return stateText[state.value]
})
const title = computed(() => {
  if (!props.el) return ''
  const p = props.el.props
  return p.label || p.text || objectName.value || (isSwitch.value ? 'Kopplingsapparat' : 'Kommando')
})
const controlTag = computed(() => props.el && config.tagMap[props.el.props.controlTag])
// descriptive object name for the dialog and the event log
const objectName = computed(() => {
  if (!props.el) return ''
  const t = config.tagMap[props.el.props.statusTag] || controlTag.value
  return t ? t.name : ''
})
const state = computed(() => (props.el ? switchState(props.el.props, live.values[props.el.props.statusTag]) : 'unknown'))
const stateColor = computed(() => ({ closed: 'error', open: 'success', intermediate: 'warning', bad: 'purple' }[state.value] || 'grey'))

watch(
  () => props.el,
  (el) => {
    phase.value = 'select'
    pending.value = null
    error.value = ''
    clearTimeout(timeout)
    if (el && el.type === 'button') setpoint.value = num(el.props.value, 0)
  },
)

function choose(kind) {
  const p = props.el.props
  if (kind === 'open') pending.value = { kind, value: num(p.openCmd, 0), text: opText.value.open, target: 'open' }
  else if (kind === 'close') pending.value = { kind, value: num(p.closeCmd, 1), text: opText.value.close, target: 'closed' }
  else {
    const v = p.askValue ? Number(setpoint.value) : num(p.value, 1)
    pending.value = { kind, value: v, text: `${p.text || 'Kommando'} (${v})` }
  }
  phase.value = 'confirm'
}

async function execute() {
  busy.value = true
  error.value = ''
  try {
    await api.command({ tag: controlTag.value.id, value: pending.value.value, source: objectName.value || title.value, text: pending.value.text, user: 'operatör' })
    notify(`Manöver skickad: ${pending.value.text} ${title.value}`)
    if (isSwitch.value && props.el.props.statusTag) {
      phase.value = 'wait'
      waiting.value = true
      success.value = false
      waitText.value = 'Väntar på lägesändring…'
      timeout = setTimeout(() => {
        waiting.value = false
        waitText.value = 'Ingen lägesändring inom 15 s – kontrollera händelselistan.'
      }, 15000)
      // the new position may already have arrived over the websocket
      // before the HTTP response did
      checkReached(state.value)
    } else {
      close()
    }
  } catch (e) {
    error.value = e.message
    phase.value = 'select'
  } finally {
    busy.value = false
  }
}

watch(state, (s) => checkReached(s))

function checkReached(s) {
  if (phase.value === 'wait' && waiting.value && pending.value && s === pending.value.target) {
    clearTimeout(timeout)
    waiting.value = false
    success.value = true
    waitText.value = `${title.value}: ${stateLabel.value}.`
    timeout = setTimeout(close, 1500)
  }
}

function close() {
  clearTimeout(timeout)
  emit('close')
}
onUnmounted(() => clearTimeout(timeout))
</script>
