<template>
  <!-- blinks until acknowledged; red while active, dark yellow once acknowledged, grey once returned to normal -->
  <v-icon
    :icon="alarm.active ? 'mdi-bell-alert' : 'mdi-bell-check-outline'"
    :color="!alarm.active ? 'grey' : alarm.acked ? '#c9a227' : 'error'"
    :class="{ 'alarm-blink': !alarm.acked }"
    :title="title"
    size="small"
  />
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({ alarm: { type: Object, required: true } })
const title = computed(() => `${props.alarm.active ? 'Aktivt' : 'Återgått'}, ${props.alarm.acked ? 'kvitterat' : 'ej kvitterat'}`)
</script>
