// Central service identity helpers. Both app.js (panel/url resolution) and
// TabBar.js (status badges/tooltips) use these so the mapping between a tab
// and its backing service lives in one place.
export const SERVICES = [
  { id: 'neuronet', label: 'neuronet', url: 'http://localhost:8000/admin', icon: 'brain' },
  { id: 'minecraft', label: 'minecraft', url: 'http://51.75.77.248:9800', icon: 'server' },
  { id: 'slotbuilder', label: 'slotbuilder', url: 'https://backoffice.7casinogames.com', icon: 'gamepad' },
]

// serviceForTab returns the service descriptor matching the tab, or null.
export function serviceForTab(tab) {
  if (!tab) return null
  const key = String(tab.url || tab.label || '').toLowerCase()
  for (const s of SERVICES) {
    if (key.includes(s.id)) return s
  }
  return null
}

// urlForTab returns the canonical external URL for a tab: the service admin
// URL for built-in services, otherwise the tab's own URL.
export function urlForTab(tab) {
  if (!tab) return null
  const svc = serviceForTab(tab)
  if (svc) return svc.url
  if (tab.url && /^https?:\/\//i.test(tab.url)) return tab.url
  return null
}

// HTTP(S) URLs open in an iframe; anything else is a built-in panel service.
export const ZOOM_MIN = 0.5
export const ZOOM_MAX = 2.5

// tabZoom returns the clamped, persisted zoom factor of a tab (default 1).
export function tabZoom(tab) {
  const z = tab && tab.settings ? Number(tab.settings.zoom) : NaN
  if (!Number.isFinite(z)) return 1
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(z * 10) / 10))
}

// statusForTab finds the health status entry matching a tab's service.
export function statusForTab(tab, statuses) {
  if (!tab) return null
  const svc = serviceForTab(tab)
  if (!svc) return null
  return (statuses || []).find(s => s.id === svc.id) || null
}