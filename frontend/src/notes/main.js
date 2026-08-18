// Notes editor window entry point (separate vite entry, served at
// "wails://wails/notes.html?tab=<id>"). This page does NOT import the Wails
// runtime/bindings — a second webview cannot use Wails IPC. Instead every Go
// call goes through the WebKit message handler "dashboardNotes" (registered in
// tabs_shell.go) and responses arrive via window.__dashReply (notes_bridge.go).
// A tab can own MULTIPLE notes: a left sidebar lists them, the right pane is
// the title + content editor. Notes are created/updated/deleted through the
// "saveNote"/"deleteNote" bridge methods.
import '../styles/main.css'
import { icon, escapeHtml } from '../components/Shared/utils.js'

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
  saveNote: (tabId, noteId, title, content) => call('saveNote', Number(tabId), Number(noteId), title || '', content || ''),
  deleteNote: (tabId, noteId) => call('deleteNote', Number(tabId), Number(noteId)),
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
        const w = Math.min(Math.max(Math.round(r.width) + 24, 420), 1200)
        const h = Math.min(Math.round(r.height) + 24, 900)
        const key = `${w}x${h}`
        if (key === lastFit) return
        lastFit = key
        notesApi.resize(w, h).catch(() => {})
      })
    })
  }, 120)
}

// Format a RFC3339 timestamp into a compact "gg/mm/aaaa" date.
function formatDate(value) {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString('it-IT', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

function snippet(note) {
  const text = (note.content || '').replace(/\s+/g, ' ').trim()
  return text.length > 60 ? text.slice(0, 60) + '…' : text
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
        <div class="notes-layout">
          <aside class="notes-sidebar">
            <div class="notes-sidebar-head">
              <span class="notes-count" id="notes-count">0 note</span>
              <button class="btn btn-icon btn-icon-soft" id="notes-new" title="Nuova nota" aria-label="Nuova nota">${icon('plus', 14)}</button>
            </div>
            <div class="notes-list" id="notes-list"></div>
          </aside>
          <section class="notes-editor">
            <input type="text" id="notes-title-input" class="notes-title-input" maxlength="120" placeholder="Titolo nota…" spellcheck="false" aria-label="Titolo nota" disabled>
            <textarea id="notes-textarea" class="notes-textarea" placeholder="Scrivi il contenuto della nota…" spellcheck="false" aria-label="Contenuto nota" disabled></textarea>
            <div class="notes-empty" id="notes-empty" hidden>${icon('note', 20)} Nessuna nota. Creane una con il pulsante +.</div>
          </section>
        </div>
        <footer class="notes-footer">
          <span class="notes-status" id="notes-status"></span>
          <span class="notes-footer-actions">
            <button class="btn btn-secondary" id="notes-delete" title="Elimina questa nota">${icon('trash', 14)} Elimina</button>
            <button class="btn btn-primary" id="notes-save">${icon('check', 14)} Salva</button>
          </span>
        </footer>
      </div>
    </div>
  `

  const listEl = app.querySelector('#notes-list')
  const countEl = app.querySelector('#notes-count')
  const titleEl = app.querySelector('.notes-title-label')
  const titleInput = app.querySelector('#notes-title-input')
  const textarea = app.querySelector('#notes-textarea')
  const emptyEl = app.querySelector('#notes-empty')
  const deleteBtn = app.querySelector('#notes-delete')
  const saveBtn = app.querySelector('#notes-save')
  const statusEl = app.querySelector('#notes-status')

  let notes = []
  // Currently edited note: {id, title, content}. id is undefined for a new
  // (unsaved) note created with the + button.
  let current = null
  let dirty = false
  let saveTimer = null
  let tabLabel = ''

  const setStatus = (text) => { statusEl.textContent = text || '' }

  const editorEnabled = (enabled) => {
    titleInput.disabled = !enabled
    textarea.disabled = !enabled
    deleteBtn.disabled = !enabled
  }

  const renderList = (selectedId) => {
    listEl.innerHTML = notes.map((n) => `
      <div class="notes-list-item${n.id === selectedId ? ' active' : ''}" data-note-id="${n.id}">
        <div class="notes-list-main">
          <div class="notes-list-title">${escapeHtml(n.title || 'Senza titolo')}</div>
          <div class="notes-list-snippet">${escapeHtml(snippet(n)) || '&nbsp;'}</div>
          <div class="notes-list-date">${formatDate(n.updated_at || n.created_at)}</div>
        </div>
        <button class="notes-del-btn" data-note-id="${n.id}" title="Elimina" aria-label="Elimina nota">${icon('trash', 12)}</button>
      </div>
    `).join('')

    countEl.textContent = notes.length === 1 ? '1 nota' : `${notes.length} note`

    listEl.querySelectorAll('.notes-list-item').forEach((item) => {
      item.addEventListener('click', (e) => {
        if (e.target.closest('.notes-del-btn')) return
        selectNote(Number(item.dataset.noteId))
      })
      item.querySelector('.notes-del-btn').addEventListener('click', (e) => {
        e.stopPropagation()
        deleteNote(Number(item.dataset.noteId))
      })
    })

    const empty = notes.length === 0
    emptyEl.hidden = !empty
    titleInput.hidden = empty
    textarea.hidden = empty
    editorEnabled(!empty)
  }

  // Flush the current note to Go (insert or update). Returns the refreshed
  // note id list state via the bridge reply (which carries the whole tab).
  const flush = async () => {
    clearTimeout(saveTimer)
    saveTimer = null
    if (!current || !dirty) return
    dirty = false
    const noteId = current.id || 0
    setStatus('Salvataggio…')
    try {
      const tab = await notesApi.saveNote(tabId, noteId, titleInput.value.trim() || 'Nota', textarea.value)
      syncFromTab(tab, true)
      setStatus('Salvato')
    } catch (error) {
      dirty = true
      setStatus('Errore di salvataggio')
      console.error('Failed to save note:', error)
    }
  }

  // Reconcile the local note list with the server's and keep the editor state.
  const syncFromTab = (tab, keepSelection) => {
    notes = (tab && tab.notes) || []
    const currentId = keepSelection && current ? current.id : undefined
    let target
    if (currentId != null) {
      target = notes.find((n) => n.id === currentId)
    }
    if (!target) {
      target = notes[notes.length - 1]
    }
    renderList(target ? target.id : null)
    if (target) {
      current = { id: target.id, title: target.title || '', content: target.content || '' }
      titleInput.value = current.title
      textarea.value = current.content
      renderList(target.id)
    } else {
      current = null
      titleInput.value = ''
      textarea.value = ''
      renderList(null)
    }
  }

  const selectNote = async (id) => {
    if (current && current.id === id) return
    if (dirty) await flush()
    const note = notes.find((n) => n.id === id)
    if (!note) return
    current = { id: note.id, title: note.title || '', content: note.content || '' }
    dirty = false
    titleInput.value = current.title
    textarea.value = current.content
    renderList(id)
    titleInput.focus()
    titleInput.select()
  }

  const newNote = () => {
    flush().then(() => {
      current = { title: '', content: '' }
      dirty = false
      titleInput.value = ''
      textarea.value = ''
      renderList(null)
      titleInput.focus()
    })
  }

  const deleteNote = async (id) => {
    if (current && current.id === id && dirty) {
      await flush().catch(() => {})
    }
    try {
      const tab = await notesApi.deleteNote(tabId, id)
      current = null
      dirty = false
      syncFromTab(tab, false)
      setStatus('Nota eliminata')
    } catch (error) {
      setStatus('Errore di eliminazione')
      console.error('Failed to delete note:', error)
    }
  }

  titleInput.addEventListener('input', () => { dirty = true; setStatus('Non salvato…'); armAutosave() })
  textarea.addEventListener('input', () => { dirty = true; setStatus('Non salvato…'); armAutosave() })
  const armAutosave = () => {
    clearTimeout(saveTimer)
    saveTimer = setTimeout(flush, 900)
  }

  saveBtn.addEventListener('click', flush)
  deleteBtn.addEventListener('click', () => {
    if (current) deleteNote(current.id)
  })
  app.querySelector('#notes-new').addEventListener('click', newNote)
  app.querySelector('#notes-close').addEventListener('click', async () => {
    clearTimeout(saveTimer)
    saveTimer = null
    if (dirty && current) {
      dirty = false
      try {
        await notesApi.saveNote(tabId, current.id || 0, titleInput.value.trim() || 'Nota', textarea.value)
      } catch { /* best effort: window closes anyway */ }
    }
    notesApi.closeNotes().catch(() => {})
  })

  try {
    const tab = await notesApi.getTab(tabId)
    tabLabel = (tab && tab.label) || ''
    if (tabLabel) titleEl.textContent = `Note — ${tabLabel}`
    syncFromTab(tab, false)
  } catch {
    syncFromTab({ notes: [] }, false)
  }
  dirty = false
  setStatus('')

  document.getElementById('loading').remove()
  scheduleFit()
  textarea.focus()
}

mount()