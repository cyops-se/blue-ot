<template>
  <v-text-field :model-value="modelValue" :label="label" class="mono" @update:model-value="$emit('update:modelValue', $event)">
    <template #prepend-inner>
      <v-menu :close-on-content-click="false">
        <template #activator="{ props: mp }">
          <div v-bind="mp" class="swatch" :style="{ background: modelValue }" />
        </template>
        <v-color-picker :model-value="modelValue" mode="hexa" :modes="['hexa']" @update:model-value="$emit('update:modelValue', $event)" />
      </v-menu>
    </template>
  </v-text-field>
</template>

<script setup>
defineProps({ modelValue: { type: String, default: '' }, label: { type: String, default: '' } })
defineEmits(['update:modelValue'])
</script>

<style scoped>
.swatch {
  width: 20px;
  height: 20px;
  border-radius: 4px;
  border: 1px solid rgba(255, 255, 255, 0.4);
  cursor: pointer;
  background-image: linear-gradient(45deg, #666 25%, transparent 25%), linear-gradient(-45deg, #666 25%, transparent 25%);
  background-size: 8px 8px;
}
</style>
