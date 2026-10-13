<template>
  <v-container fluid class="fill-page">
    <div class="d-flex align-center mb-4">
      <h2 class="text-h6">Vyer</h2>
      <v-spacer />
      <v-btn color="primary" prepend-icon="mdi-plus" @click="create">Ny vy</v-btn>
    </div>
    <v-row>
      <v-col v-for="v in views" :key="v.id" cols="12" sm="6" md="4" lg="3">
        <v-card :to="`/views/${v.id}`" border>
          <v-card-title class="d-flex align-center">
            <v-icon icon="mdi-monitor-dashboard" class="mr-2" />{{ v.name }}
          </v-card-title>
          <v-card-subtitle>Ändrad {{ new Date(v.updated).toLocaleString('sv-SE') }}</v-card-subtitle>
          <v-card-actions>
            <v-btn variant="text" prepend-icon="mdi-play" :to="`/views/${v.id}`">Öppna</v-btn>
            <v-btn variant="text" prepend-icon="mdi-pencil" :to="`/views/${v.id}/edit`" @click.stop>Redigera</v-btn>
            <v-spacer />
            <v-btn icon="mdi-delete" variant="text" size="small" @click.prevent.stop="remove(v)" />
          </v-card-actions>
        </v-card>
      </v-col>
      <v-col v-if="!views.length" cols="12">
        <v-alert type="info" variant="tonal">Inga vyer ännu. Skapa en ny vy för att börja.</v-alert>
      </v-col>
    </v-row>
  </v-container>
</template>

<script setup>
import { inject, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'

const views = ref([])
const router = useRouter()
const notify = inject('notify')

const load = async () => (views.value = await api.views())

async function create() {
  const v = await api.createView({ name: 'Ny vy', width: 1600, height: 900, grid: 10, background: '#0d1117', elements: [] })
  router.push(`/views/${v.id}/edit`)
}

async function remove(v) {
  if (!confirm(`Ta bort vyn "${v.name}"?`)) return
  await api.deleteView(v.id)
  notify('Vyn borttagen')
  load()
}

onMounted(load)
</script>
