// Thin wrapper around fetch for the backend REST API.

async function request(method, url, body) {
  const opts = { method, headers: {} }
  if (body instanceof FormData) {
    opts.body = body
  } else if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(url, opts)
  if (res.status === 204) return null
  const data = await res.json().catch(() => null)
  if (!res.ok || (data && data.ok === false)) {
    throw new Error((data && data.error) || `${res.status} ${res.statusText}`)
  }
  return data
}

const enc = encodeURIComponent

export const api = {
  config: () => request('GET', '/api/config'),
  saveDevice: (oldId, d) => (oldId ? request('PUT', `/api/devices/${enc(oldId)}`, d) : request('POST', '/api/devices', d)),
  deleteDevice: (id) => request('DELETE', `/api/devices/${enc(id)}`),
  saveTag: (oldId, t) => (oldId ? request('PUT', `/api/tags/${enc(oldId)}`, t) : request('POST', '/api/tags', t)),
  deleteTag: (id) => request('DELETE', `/api/tags/${enc(id)}`),

  history: (tag, minutes = 10) => request('GET', `/api/history/${enc(tag)}?minutes=${minutes}`),
  events: () => request('GET', '/api/events'),
  command: (cmd) => request('POST', '/api/commands', cmd),

  views: () => request('GET', '/api/views'),
  view: (id) => request('GET', `/api/views/${enc(id)}`),
  createView: (v) => request('POST', '/api/views', v),
  saveView: (v) => request('PUT', `/api/views/${enc(v.id)}`, v),
  deleteView: (id) => request('DELETE', `/api/views/${enc(id)}`),

  images: () => request('GET', '/api/images'),
  uploadImage: (file) => {
    const fd = new FormData()
    fd.append('file', file)
    return request('POST', '/api/images', fd)
  },
}
