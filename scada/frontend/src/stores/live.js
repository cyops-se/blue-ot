// Realtime state fed by the backend WebSocket.
import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { api } from '../lib/api'

export const useLive = defineStore('live', () => {
  const connected = ref(false)
  const values = reactive({}) // tag -> { v, q, ts }
  const devices = reactive({}) // device -> status
  const events = ref([])
  const unseenAlarms = ref(0)
  let ws = null
  let retry = 1000

  function applyValues(list) {
    for (const v of list || []) values[v.tag] = { v: v.v, q: v.q, ts: v.ts }
  }

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    ws = new WebSocket(`${proto}://${location.host}/ws`)
    ws.onopen = () => {
      connected.value = true
      retry = 1000
      api.events().then((e) => (events.value = e)).catch(() => {})
    }
    ws.onclose = () => {
      connected.value = false
      for (const k of Object.keys(values)) values[k] = { ...values[k], q: 'bad' }
      setTimeout(connect, retry)
      retry = Math.min(retry * 2, 10000)
    }
    ws.onmessage = (ev) => {
      const m = JSON.parse(ev.data)
      switch (m.type) {
        case 'snapshot':
          applyValues(m.values)
          for (const d of m.devices || []) devices[d.device] = d
          break
        case 'values':
          applyValues(m.values)
          break
        case 'device':
          devices[m.device.device] = m.device
          break
        case 'event':
          events.value.unshift(m.event)
          if (events.value.length > 1000) events.value.length = 1000
          if (m.event.severity === 'alarm') unseenAlarms.value++
          break
      }
    }
  }

  function start() {
    if (!ws) connect()
  }

  return { connected, values, devices, events, unseenAlarms, start }
})
