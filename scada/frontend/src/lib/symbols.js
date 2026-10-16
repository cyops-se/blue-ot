// Symbol library: defaults and property schema for every element type.
// The property panel in the editor is generated from `fields`.

const dynFields = [
  { key: 'dynTag', label: 'Dynamisk färg – tagg', type: 'tag' },
  { key: 'dynRules', label: 'Färgregler', type: 'rules' },
]

const switchFields = (openDef, closedDef) => [
  { key: 'label', label: 'Etikett', type: 'text' },
  { key: 'showLabel', label: 'Visa etikett', type: 'bool' },
  { section: 'Läge (indikering)' },
  { key: 'statusTag', label: 'Lägestagg', type: 'tag' },
  { key: 'openValue', label: `Värde = öppen (t.ex. ${openDef})`, type: 'number' },
  { key: 'closedValue', label: `Värde = sluten (t.ex. ${closedDef})`, type: 'number' },
  { key: 'colorClosed', label: 'Färg sluten', type: 'color' },
  { key: 'colorOpen', label: 'Färg öppen', type: 'color' },
  { section: 'Manöver' },
  { key: 'controlTag', label: 'Manövertagg (skrivbar)', type: 'tag', writable: true },
  { key: 'openCmd', label: 'Värde för frånslag/öppna', type: 'number' },
  { key: 'closeCmd', label: 'Värde för tillslag/slut', type: 'number' },
]

export const catalog = {
  // ---------- static graphics ----------
  line: {
    label: 'Linje', group: 'Statisk grafik', icon: 'mdi-vector-line', w: 120, h: 0, line: true,
    props: { stroke: '#8fa3b8', strokeWidth: 3, dash: '', dynTag: '', dynRules: [] },
    fields: [
      { key: 'stroke', label: 'Linjefärg', type: 'color' },
      { key: 'strokeWidth', label: 'Linjebredd', type: 'number' },
      { key: 'dash', label: 'Streckmönster (t.ex. 6 4)', type: 'text' },
      ...dynFields,
    ],
  },
  busbar: {
    label: 'Samlingsskena', group: 'Statisk grafik', icon: 'mdi-minus-thick', w: 400, h: 0, line: true,
    props: { stroke: '#d0d7de', strokeWidth: 8, dash: '', dynTag: '', dynRules: [] },
    fields: [
      { key: 'stroke', label: 'Färg', type: 'color' },
      { key: 'strokeWidth', label: 'Tjocklek', type: 'number' },
      ...dynFields,
    ],
  },
  rect: {
    label: 'Rektangel', group: 'Statisk grafik', icon: 'mdi-rectangle-outline', w: 160, h: 100,
    props: { fill: '#00000000', stroke: '#5c6b7a', strokeWidth: 2, radius: 4, dash: '', dynTag: '', dynRules: [] },
    fields: [
      { key: 'fill', label: 'Fyllnad', type: 'color' },
      { key: 'stroke', label: 'Kantfärg', type: 'color' },
      { key: 'strokeWidth', label: 'Kantbredd', type: 'number' },
      { key: 'radius', label: 'Hörnradie', type: 'number' },
      { key: 'dash', label: 'Streckmönster', type: 'text' },
      ...dynFields,
    ],
  },
  ellipse: {
    label: 'Ellips', group: 'Statisk grafik', icon: 'mdi-circle-outline', w: 80, h: 80,
    props: { fill: '#00000000', stroke: '#8fa3b8', strokeWidth: 2, dynTag: '', dynRules: [] },
    fields: [
      { key: 'fill', label: 'Fyllnad', type: 'color' },
      { key: 'stroke', label: 'Kantfärg', type: 'color' },
      { key: 'strokeWidth', label: 'Kantbredd', type: 'number' },
      ...dynFields,
    ],
  },
  text: {
    label: 'Text', group: 'Statisk grafik', icon: 'mdi-format-text', w: 160, h: 30,
    props: { text: 'Text', fontSize: 18, color: '#e6edf3', bold: false, align: 'start', dynTag: '', dynRules: [] },
    fields: [
      { key: 'text', label: 'Text', type: 'text' },
      { key: 'fontSize', label: 'Teckenstorlek', type: 'number' },
      { key: 'color', label: 'Färg', type: 'color' },
      { key: 'bold', label: 'Fetstil', type: 'bool' },
      { key: 'align', label: 'Justering', type: 'select', options: [{ title: 'Vänster', value: 'start' }, { title: 'Centrerad', value: 'middle' }, { title: 'Höger', value: 'end' }] },
      ...dynFields,
    ],
  },
  image: {
    label: 'Bild', group: 'Statisk grafik', icon: 'mdi-image-outline', w: 200, h: 150,
    props: { src: '', fit: 'xMidYMid meet', opacity: 1 },
    fields: [
      { key: 'src', label: 'Bild', type: 'image' },
      { key: 'fit', label: 'Skalning', type: 'select', options: [{ title: 'Anpassa', value: 'xMidYMid meet' }, { title: 'Fyll', value: 'xMidYMid slice' }, { title: 'Sträck', value: 'none' }] },
      { key: 'opacity', label: 'Opacitet (0-1)', type: 'number' },
    ],
  },
  transformer: {
    label: 'Transformator', group: 'Statisk grafik', icon: 'mdi-circle-double', w: 50, h: 80,
    props: { stroke: '#d0d7de', strokeWidth: 3, dynTag: '', dynRules: [] },
    fields: [
      { key: 'stroke', label: 'Färg', type: 'color' },
      { key: 'strokeWidth', label: 'Linjebredd', type: 'number' },
      ...dynFields,
    ],
  },
  ground: {
    label: 'Jord', group: 'Statisk grafik', icon: 'mdi-current-dc', w: 30, h: 30,
    props: { stroke: '#8fa3b8', strokeWidth: 2 },
    fields: [
      { key: 'stroke', label: 'Färg', type: 'color' },
      { key: 'strokeWidth', label: 'Linjebredd', type: 'number' },
    ],
  },

  // ---------- dynamic symbols ----------
  breaker: {
    label: 'Brytare', group: 'Symboler', icon: 'mdi-square-outline', w: 40, h: 40, interactive: true,
    props: { label: 'Q1', showLabel: true, statusTag: '', openValue: 1, closedValue: 2, controlTag: '', openCmd: 1, closeCmd: 2, colorClosed: '#ff4d4d', colorOpen: '#2ecc71' },
    fields: switchFields(1, 2),
  },
  disconnector: {
    label: 'Frånskiljare', group: 'Symboler', icon: 'mdi-electric-switch', w: 40, h: 60, interactive: true,
    props: { label: 'QS1', showLabel: true, statusTag: '', openValue: 0, closedValue: 1, controlTag: '', openCmd: 0, closeCmd: 1, colorClosed: '#ff4d4d', colorOpen: '#2ecc71' },
    fields: switchFields(0, 1),
  },
  toggle: {
    label: 'Omkopplare', group: 'Symboler', icon: 'mdi-toggle-switch-outline', w: 200, h: 28, interactive: true,
    props: { label: 'Auto', statusTag: '', openValue: 0, closedValue: 1, controlTag: '', openCmd: 0, closeCmd: 1, offText: 'AV', onText: 'PÅ', colorClosed: '#3d8bfd', colorOpen: '#3a4450' },
    fields: [
      { key: 'label', label: 'Etikett', type: 'text' },
      { section: 'Läge (indikering)' },
      { key: 'statusTag', label: 'Lägestagg', type: 'tag' },
      { key: 'openValue', label: 'Värde = AV', type: 'number' },
      { key: 'closedValue', label: 'Värde = PÅ', type: 'number' },
      { key: 'offText', label: 'Text AV', type: 'text' },
      { key: 'onText', label: 'Text PÅ', type: 'text' },
      { key: 'colorClosed', label: 'Färg PÅ', type: 'color' },
      { key: 'colorOpen', label: 'Färg AV', type: 'color' },
      { section: 'Manöver' },
      { key: 'controlTag', label: 'Manövertagg (skrivbar)', type: 'tag', writable: true },
      { key: 'openCmd', label: 'Värde för AV', type: 'number' },
      { key: 'closeCmd', label: 'Värde för PÅ', type: 'number' },
    ],
  },
  pump: {
    label: 'Pump', group: 'Symboler', icon: 'mdi-pump', w: 40, h: 40, interactive: true,
    props: { label: 'P1', showLabel: true, statusTag: '', openValue: 0, closedValue: 1, controlTag: '', openCmd: 0, closeCmd: 1, offText: 'STOPP', onText: 'START', colorClosed: '#2ecc71', colorOpen: '#5c6b7a' },
    fields: [
      { key: 'label', label: 'Etikett', type: 'text' },
      { key: 'showLabel', label: 'Visa etikett', type: 'bool' },
      { section: 'Läge (indikering)' },
      { key: 'statusTag', label: 'Lägestagg', type: 'tag' },
      { key: 'openValue', label: 'Värde = stopp', type: 'number' },
      { key: 'closedValue', label: 'Värde = drift', type: 'number' },
      { key: 'offText', label: 'Text stopp', type: 'text' },
      { key: 'onText', label: 'Text drift', type: 'text' },
      { key: 'colorClosed', label: 'Färg drift', type: 'color' },
      { key: 'colorOpen', label: 'Färg stopp', type: 'color' },
      { section: 'Manöver' },
      { key: 'controlTag', label: 'Manövertagg (skrivbar)', type: 'tag', writable: true },
      { key: 'openCmd', label: 'Värde för stopp', type: 'number' },
      { key: 'closeCmd', label: 'Värde för drift', type: 'number' },
    ],
  },
  valve: {
    label: 'Ventil', group: 'Symboler', icon: 'mdi-valve', w: 40, h: 24, interactive: true,
    props: { label: 'V1', showLabel: true, statusTag: '', openValue: 0, closedValue: 1, controlTag: '', openCmd: 0, closeCmd: 1, offText: 'STÄNG', onText: 'ÖPPNA', colorClosed: '#2ecc71', colorOpen: '#ff4d4d' },
    fields: [
      { key: 'label', label: 'Etikett', type: 'text' },
      { key: 'showLabel', label: 'Visa etikett', type: 'bool' },
      { section: 'Läge (indikering)' },
      { key: 'statusTag', label: 'Lägestagg', type: 'tag' },
      { key: 'openValue', label: 'Värde = stängd', type: 'number' },
      { key: 'closedValue', label: 'Värde = öppen', type: 'number' },
      { key: 'offText', label: 'Text stängd', type: 'text' },
      { key: 'onText', label: 'Text öppen', type: 'text' },
      { key: 'colorClosed', label: 'Färg öppen', type: 'color' },
      { key: 'colorOpen', label: 'Färg stängd', type: 'color' },
      { section: 'Manöver' },
      { key: 'controlTag', label: 'Manövertagg (skrivbar)', type: 'tag', writable: true },
      { key: 'openCmd', label: 'Värde för stängd', type: 'number' },
      { key: 'closeCmd', label: 'Värde för öppen', type: 'number' },
    ],
  },
  measurement: {
    label: 'Mätvärde', group: 'Symboler', icon: 'mdi-numeric', w: 170, h: 32, interactive: true,
    props: { tag: '', label: '', unit: '', decimals: '', fontSize: 16, color: '#e6edf3', background: '#0d1117', border: '#30363d', lowLimit: '', highLimit: '', alarmColor: '#ff4d4d' },
    fields: [
      { key: 'tag', label: 'Tagg', type: 'tag' },
      { key: 'label', label: 'Etikett (tom = ingen)', type: 'text' },
      { key: 'unit', label: 'Enhet (tom = från tagg)', type: 'text' },
      { key: 'decimals', label: 'Decimaler (tom = från tagg)', type: 'number' },
      { key: 'fontSize', label: 'Teckenstorlek', type: 'number' },
      { key: 'color', label: 'Textfärg', type: 'color' },
      { key: 'background', label: 'Bakgrund', type: 'color' },
      { key: 'border', label: 'Kant', type: 'color' },
      { section: 'Gränsvärden' },
      { key: 'lowLimit', label: 'Undre gräns', type: 'number' },
      { key: 'highLimit', label: 'Övre gräns', type: 'number' },
      { key: 'alarmColor', label: 'Larmfärg', type: 'color' },
    ],
  },
  indicator: {
    label: 'Indikering', group: 'Symboler', icon: 'mdi-circle-slice-8', w: 24, h: 24,
    props: { tag: '', onValue: 1, onColor: '#ff4d4d', offColor: '#3a4450', label: '', blink: false },
    fields: [
      { key: 'tag', label: 'Tagg', type: 'tag' },
      { key: 'onValue', label: 'Värde = aktiv', type: 'number' },
      { key: 'onColor', label: 'Färg aktiv', type: 'color' },
      { key: 'offColor', label: 'Färg inaktiv', type: 'color' },
      { key: 'label', label: 'Etikett', type: 'text' },
      { key: 'blink', label: 'Blinka när aktiv', type: 'bool' },
    ],
  },
  bar: {
    label: 'Stapel', group: 'Symboler', icon: 'mdi-chart-bar', w: 30, h: 120, interactive: true,
    props: { tag: '', min: 0, max: 100, color: '#3d8bfd', background: '#0d1117', showValue: true },
    fields: [
      { key: 'tag', label: 'Tagg', type: 'tag' },
      { key: 'min', label: 'Min', type: 'number' },
      { key: 'max', label: 'Max', type: 'number' },
      { key: 'color', label: 'Färg', type: 'color' },
      { key: 'background', label: 'Bakgrund', type: 'color' },
      { key: 'showValue', label: 'Visa värde', type: 'bool' },
    ],
  },
  button: {
    label: 'Kommandoknapp', group: 'Symboler', icon: 'mdi-gesture-tap-button', w: 140, h: 36, interactive: true,
    props: { text: 'Kommando', controlTag: '', value: 1, askValue: false, color: '#3d8bfd', textColor: '#ffffff' },
    fields: [
      { key: 'text', label: 'Text', type: 'text' },
      { key: 'controlTag', label: 'Manövertagg (skrivbar)', type: 'tag', writable: true },
      { key: 'value', label: 'Värde att skriva', type: 'number' },
      { key: 'askValue', label: 'Fråga efter värde (börvärde)', type: 'bool' },
      { key: 'color', label: 'Färg', type: 'color' },
      { key: 'textColor', label: 'Textfärg', type: 'color' },
    ],
  },
  link: {
    label: 'Vy-länk', group: 'Symboler', icon: 'mdi-link-variant', w: 140, h: 36, interactive: true,
    props: { text: 'Till vy', view: '', color: '#30363d', textColor: '#e6edf3' },
    fields: [
      { key: 'text', label: 'Text', type: 'text' },
      { key: 'view', label: 'Vy', type: 'view' },
      { key: 'color', label: 'Färg', type: 'color' },
      { key: 'textColor', label: 'Textfärg', type: 'color' },
    ],
  },
}

export const groups = ['Statisk grafik', 'Symboler']

let seq = 0
export function newElement(type, x, y) {
  const def = catalog[type]
  return {
    id: `${type}-${Date.now().toString(36)}-${(seq++).toString(36)}`,
    type,
    x: Math.round(x),
    y: Math.round(y),
    w: def.w,
    h: def.h,
    rot: 0,
    props: JSON.parse(JSON.stringify(def.props)),
  }
}

// num treats '' / null / undefined as "not set".
export function num(v, fallback) {
  if (v === '' || v === null || v === undefined) return fallback
  const n = Number(v)
  return Number.isFinite(n) ? n : fallback
}

// switchState maps a status value to closed/open/intermediate/bad.
export function switchState(props, value) {
  if (!props.statusTag) return 'unknown'
  if (!value || value.q !== 'good') return 'bad'
  if (value.v === num(props.closedValue, 1)) return 'closed'
  if (value.v === num(props.openValue, 0)) return 'open'
  return 'intermediate'
}

export const stateText = {
  closed: 'Sluten',
  open: 'Öppen',
  intermediate: 'Mellanläge / fel',
  bad: 'Osäkert värde',
  unknown: 'Ej kopplad',
}

const ops = {
  '==': (a, b) => a === b,
  '!=': (a, b) => a !== b,
  '>': (a, b) => a > b,
  '>=': (a, b) => a >= b,
  '<': (a, b) => a < b,
  '<=': (a, b) => a <= b,
}
export const ruleOps = Object.keys(ops)

// dynColor returns the color of the first matching rule, or null.
export function dynColor(props, value) {
  if (!props.dynTag || !Array.isArray(props.dynRules)) return null
  if (!value || value.q !== 'good') return '#b04dff'
  for (const r of props.dynRules) {
    const f = ops[r.op]
    if (f && f(value.v, Number(r.value))) return r.color
  }
  return null
}

export function formatValue(value, tag, decimals) {
  if (!value) return '---'
  if (value.q !== 'good') return '???'
  const d = num(decimals, tag ? tag.decimals : 2)
  return value.v.toLocaleString('sv-SE', { minimumFractionDigits: d, maximumFractionDigits: d })
}
