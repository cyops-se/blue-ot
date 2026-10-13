// Communication configuration (devices and tags).
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '../lib/api'

export const useConfig = defineStore('config', () => {
  const devices = ref([])
  const tags = ref([])
  const loaded = ref(false)

  const tagMap = computed(() => Object.fromEntries(tags.value.map((t) => [t.id, t])))

  async function load() {
    const c = await api.config()
    devices.value = c.devices || []
    tags.value = c.tags || []
    loaded.value = true
  }

  function ensure() {
    return loaded.value ? Promise.resolve() : load()
  }

  return { devices, tags, tagMap, loaded, load, ensure }
})
