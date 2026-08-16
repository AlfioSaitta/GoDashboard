// Notes editor window entry point (separate vite entry, served at
// "wails://wails/notes.html?tab=<id>"). This page does NOT import the Wails
// runtime/bindings — a second webview cannot use Wails IPC. Instead every Go
// call goes through the WebKit message handler "dashboardNotes" (registered in
// tabs_shell.go) and responses arrive via window.__dashReply (notes_bridge.go).
import '../styles/main.css'
import { icon } from '../components/Shared/utils.js'

const pending = new Map()
let seq = 0

function call(method, ...args) {
  return new Promise((resolve, reject) => {
    const id = ++seq
    pending.set(id, { resolve, reject })
    window.webkit.messageHandlers.dashboardNotes.postMessage(
      JSON.stringify({ id, method, args }),
    )
  })
}

window.__dashReply = (payload) => {
  const { id, ok, result, error } = payload || {}
  const entry = pending.get(id)
  if (!entry) return
  pending.delete(id)
  if (ok) entry.resolve(result)
  else entry.reject(new Error(error || 'Errore bridge'))
}

const notesApi = {
  getTab: (id) => call('getTab', Number(id)),
  getNotes: (id) => call('getNotes', Number(id)),
  saveNotes: (id, notes) => call('saveNotes', Number(id), notes || ''),
  getTheme: () => call('getTheme'),
  getSystemTheme: () => call('getSystemTheme'),
  closeNotes: () => call('closeNotes'),
  resize: (w, h) => call('resize', Math.round(w), Math.round(h)),
}

// ── Theme (mirrors the settings-window logic) ─────────────
function applyTheme() {
  return notesApi
    .getTheme()
    .catch(() => 'system')
    .then((pref) => {
      if (pref === 'system') {
        return notesApi
          .getSystemTheme()
          .then((sys) => { document.documentElement.dataset.theme = sys === 'light' ? 'light' : 'dark' })
          .catch(() => { document.documentElement.dataset.theme = 'dark' })
      }
      document.documentElement.dataset.theme = pref === 'light' ? 'light' : 'dark'
    })
}

const tabId = Number(new URLSearchParams(window.location.search).get('tab') || 0)

// Sizes the native (transparent) window to the notes card, once layout is done.
let fitTimer = 0
let lastFit = ''
function scheduleFit() {
  clearTimeout(fitTimer)
  fitTimer = window.setTimeout(() => {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const card = document.querySelector('.notes-card')
        if (!card) return
        const r = card.getBoundingClientRect()
        const w = Math.min(Math.max(Math.round(r.width) + 24, 320), 1200)
        const h = Math.min(Math.round(r.height) + 24, 900)
        const key = `${w}x${h}`
        if (key === lastFit) return
        lastFit = key
        notesApi.resize(w, h).catch(() => {})
      })
    })
  }, 120)
}

async function mount() {
  try {
    await applyTheme()
  } catch { /* keep default */ }

  const app = document.getElementById('app')
  document.body.classList.add('notes-mode')
  if (new URLSearchParams(window.location.search).get('t') === '1') {
    document.body.classList.add('dash-transparent')
  }

  app.innerHTML = `
    <div class="notes-overlay">
      <div class="notes-card" role="dialog" aria-labelledby="notes-title">
        <header class="notes-header">
          <h2 id="notes-title">${icon('note', 16)} <span class="notes-title-label">Note</span></h2>
          <button class="btn btn-icon btn-close" id="notes-close" aria-label="Chiudi" title="Chiudi">${icon('close', 18)}</button>
        </header>
        <div class="notes-body">
          <textarea id="notes-textarea" class="notes-textarea" placeholder="Scrivi qui le note persistenti per questo tab…" spellcheck="false" aria-label="Note del tab"></textarea>
        </div>
        <footer class="notes-footer">
          <span class="notes-status" id="notes-status"></span>
          <span class="notes-footer-actions">
            <button class="btn btn-secondary" id="notes-clear" title="Elimina tutte le note">${icon('refresh', 14)} Svuota</button>
            <button class="btn btn-primary" id="notes-save">${icon('check', 14)} Salva</button>
          </span>
        </footer>
      </div>
    </div>
  `

  const textarea = app.querySelector('#notes-textarea')
  const statusEl = app.querySelector('#notes-status')
  const titleEl = app.querySelector('.notes-title-label')
  let saveTimer = null
  let dirty = false

  const setStatus = (text) => { statusEl.textContent = text || '' }

  const flush = async () => {
    if (dirty === false && saveTimer == null) return
    clearTimeout(saveTimer)
    saveTimer = null
    if (!dirty) return
    dirty = false
    const notes = textarea.value
    setStatus('Salvataggio…')
    try {
      await notesApi.saveNotes(tabId, notes)
      setStatus('Salvato')
    } catch (error) {
      dirty = true
      setStatus('Errore di salvataggio')
      console.error('Failed to save notes:', error)
    }
  }

  const renderTab = (tab) => {
    if (tab && tab.label) titleEl.textContent = `Note — ${tab.label}`
  }

  textarea.addEventListener('input', () => {
    dirty = true
    setStatus('Non salvato…')
    clearTimeout(saveTimer)
    saveTimer = setTimeout(flush, 900)
  })
  app.querySelector('#notes-save').addEventListener('click', flush)
  app.querySelector('#notes-clear').addEventListener('click', () => {
    textarea.value = ''
    dirty = true
    setStatus('')
    clearTimeout(saveTimer)
    flush()
  })
  app.querySelector('#notes-close').addEventListener('click', () => {
    clearTimeout(saveTimer)
    notesApi.closeNotes().catch(() => {})
  })

  try {
    const tab = await notesApi.getTab(tabId)
    renderTab(tab)
    textarea.value = (tab && tab.notes) || ''
  } catch {
    textarea.value = ''
  }
  dirty = false
  setStatus('')

  document.getElementById('loading').remove()
  scheduleFit()
  textarea.focus()
}

mount()