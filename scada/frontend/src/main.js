import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import { createVuetify } from 'vuetify'
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import './style.css'

import App from './App.vue'
import ViewsList from './pages/ViewsList.vue'
import ViewRuntime from './pages/ViewRuntime.vue'
import ViewEditor from './pages/ViewEditor.vue'
import ValuesPage from './pages/ValuesPage.vue'
import EventsPage from './pages/EventsPage.vue'
import ConfigPage from './pages/ConfigPage.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/views' },
    { path: '/views', component: ViewsList },
    { path: '/views/:id', component: ViewRuntime, props: true },
    { path: '/views/:id/edit', component: ViewEditor, props: true },
    { path: '/values', component: ValuesPage },
    { path: '/events', component: EventsPage },
    { path: '/config', component: ConfigPage },
  ],
})

const vuetify = createVuetify({
  theme: {
    defaultTheme: 'scadaDark',
    themes: {
      scadaDark: {
        dark: true,
        colors: {
          background: '#11161c',
          surface: '#1a2129',
          primary: '#3d8bfd',
          secondary: '#5c6b7a',
          error: '#ff5252',
          warning: '#ffb300',
          success: '#2ecc71',
        },
      },
    },
  },
  defaults: {
    VTextField: { density: 'compact', variant: 'outlined', hideDetails: 'auto' },
    VSelect: { density: 'compact', variant: 'outlined', hideDetails: 'auto' },
    VAutocomplete: { density: 'compact', variant: 'outlined', hideDetails: 'auto' },
    VCheckbox: { density: 'compact', hideDetails: true },
    VSwitch: { density: 'compact', hideDetails: true, color: 'primary' },
  },
})

createApp(App).use(createPinia()).use(router).use(vuetify).mount('#app')
