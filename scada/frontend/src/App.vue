<template>
  <v-app>
    <v-navigation-drawer v-model="drawer" rail expand-on-hover permanent>
      <v-list density="compact" nav>
        <v-list-item prepend-icon="mdi-transmission-tower" title="Blue OT SCADA" class="mb-2" />
        <v-divider class="mb-2" />
        <v-list-item to="/views" prepend-icon="mdi-monitor-dashboard" title="Vyer" />
        <v-list-item to="/values" prepend-icon="mdi-table" title="Mätvärden" />
        <v-list-item to="/events" prepend-icon="mdi-format-list-bulleted" title="Händelser">
          <template #append>
            <v-badge v-if="live.unseenAlarms" :content="live.unseenAlarms" color="error" inline />
          </template>
        </v-list-item>
        <v-list-item to="/config" prepend-icon="mdi-cog" title="Konfiguration" />
      </v-list>
    </v-navigation-drawer>

    <v-app-bar density="compact" flat border>
      <v-app-bar-title class="text-body-1">{{ title }}</v-app-bar-title>
      <template #append>
        <v-chip
          v-for="d in deviceList"
          :key="d.device"
          size="small"
          class="mr-2"
          :color="d.connected ? 'success' : 'error'"
          :prepend-icon="d.connected ? 'mdi-lan-connect' : 'mdi-lan-disconnect'"
          :title="d.error || 'OK'"
        >{{ d.device }}</v-chip>
        <v-chip size="small" class="mr-2" :color="live.connected ? 'success' : 'error'" variant="flat">
          {{ live.connected ? 'Online' : 'Offline' }}
        </v-chip>
        <span class="mono text-caption mr-4">{{ clock }}</span>
      </template>
    </v-app-bar>

    <v-main>
      <router-view />
    </v-main>

    <v-snackbar v-model="toast.show" :color="toast.color" location="bottom right" timeout="4000">
      {{ toast.text }}
    </v-snackbar>
  </v-app>
</template>

<script setup>
import { computed, onMounted, onUnmounted, provide, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useLive } from './stores/live'
import { useConfig } from './stores/config'

const live = useLive()
const config = useConfig()
const route = useRoute()
const drawer = ref(true)
const clock = ref('')

const toast = reactive({ show: false, text: '', color: 'success' })
provide('notify', (text, color = 'success') => Object.assign(toast, { show: true, text, color }))

const deviceList = computed(() => Object.values(live.devices).sort((a, b) => a.device.localeCompare(b.device)))
const title = computed(() => {
  if (route.path.startsWith('/views/') && route.path.endsWith('/edit')) return 'Vy-editor'
  if (route.path.startsWith('/views/')) return 'Driftbild'
  return { '/views': 'Vyer', '/values': 'Mätvärden', '/events': 'Händelser', '/config': 'Konfiguration' }[route.path] || ''
})

let timer
onMounted(() => {
  live.start()
  config.ensure()
  const tick = () => (clock.value = new Date().toLocaleString('sv-SE'))
  tick()
  timer = setInterval(tick, 1000)
})
onUnmounted(() => clearInterval(timer))
</script>
