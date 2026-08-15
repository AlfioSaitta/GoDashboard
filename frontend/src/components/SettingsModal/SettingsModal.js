import { createElement, icon, escapeHtml } from '../Shared/utils.js'
import { serviceForTab, tabZoom } from '../Shared/services.js'

const ICONS = [
  ['brain', 'Cervello (AI/ML)'],
  ['server', 'Server'],
  ['gamepad', 'Gamepad'],
  ['database', 'Database'],
  ['chart', 'Grafico'],
  ['rocket', 'Rocket'],
  ['activity', 'Attività'],
  ['star', 'Stella'],
  ['edit', 'Modifica'],
  ['copy', 'Copia'],
  ['external', 'Esterno'],
  ['check', 'Spunta'],
  ['refresh', 'Aggiorna'],
]

export class SettingsModal {
  constructor(overlayEl, { onSave, onUpdateTab, onRemoveTab, onClose, onReorder, onThemeChange, onUpdateSettings }) {
    this.overlay = overlayEl
    this.onSave = onSave
    this.onUpdateTab = onUpdateTab
    this.onRemoveTab = onRemoveTab
    this.onClose = onClose
    this.onReorder = onReorder
    this.onThemeChange = onThemeChange
    this.onUpdateSettings = onUpdateSettings
    this.tabs = []
    this.editingId = null
    this.draggingId = null
    this.removeCandidateId = null
    this.settingsTabId = null
    this.render()
  }

  render() {
    this.overlay.innerHTML = `
      <div class="modal" role="document">
        <header class="modal-header">
          <h2 id="settings-title">${icon('settings', 20)} Impostazioni</h2>
          <button class="btn btn-icon btn-close" id="close-modal" aria-label="Chiudi">${icon('close', 18)}</button>
        </header>
        <div class="modal-body">
          <section class="settings-section">
            <h3 id="form-section-title">Aggiungi Nuovo Tab</h3>
            <form id="add-tab-form" class="form">
              <div class="form-row">
                <div class="form-group">
                  <label for="tab-label">Etichetta</label>
                  <input type="text" id="tab-label" name="label" placeholder="Es. Mio Servizio" required autocomplete="off">
                </div>
                <div class="form-group">
                  <label for="tab-icon">Icona</label>
                  <select id="tab-icon" name="icon">
                    ${ICONS.map(([value, label]) => `<option value="${value}">${label}</option>`).join('')}
                  </select>
                </div>
              </div>
              <div class="form-group">
                <label for="tab-url">URL</label>
                <input type="url" id="tab-url" name="url" placeholder="https://example.com" autocomplete="off">
              </div>
              <div class="form-actions">
                <button type="button" class="btn btn-secondary" id="cancel-edit-btn" style="display:none">Annulla</button>
                <button type="submit" class="btn btn-primary" id="submit-tab-btn">${icon('plus', 14)} Aggiungi</button>
              </div>
            </form>
          </section>

          <section class="settings-section">
            <h3>Tab Configurati</h3>
            <ul id="tab-list" class="tab-list" aria-label="Elenco tab"></ul>
          </section>

          <section class="settings-section">
            <h3>Tema</h3>
            <div class="form-group">
              <label for="theme-select">Aspetto</label>
              <select id="theme-select" name="theme">
                <option value="system">Automatico (sistema)</option>
                <option value="dark">Scuro</option>
                <option value="light">Chiaro</option>
              </select>
            </div>
          </section>

          <div class="modal-error" id="modal-error" role="alert" hidden></div>
        </div>
        <footer class="modal-footer">
          <button class="btn btn-secondary" id="close-modal-footer">Chiudi</button>
        </footer>
      </div>
    `

    this.overlay.querySelector('#close-modal').addEventListener('click', () => this.close())
    this.overlay.querySelector('#close-modal-footer').addEventListener('click', () => this.close())
    this.overlay.addEventListener('mousedown', (e) => {
      if (e.target === this.overlay) this.close()
    })
    this.overlay.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        this.close()
      }
    })

    this.overlay.querySelector('#cancel-edit-btn').addEventListener('click', () => this.cancelEdit())
    this.overlay.querySelector('#add-tab-form').addEventListener('submit', (e) => this.handleSubmit(e))
    this.overlay.querySelector('#theme-select').addEventListener('change', (e) => {
      this.hideError()
      if (this.onThemeChange) this.onThemeChange(e.target.value)
    })
  }

  setTheme(theme) {
    const select = this.overlay.querySelector('#theme-select')
    if (select && ['system', 'dark', 'light'].includes(theme)) select.value = theme
  }

  open(tabs) {
    this.tabs = tabs || []
    this.editingId = null
    this.removeCandidateId = null
    this.settingsTabId = null
    this.setFormMode(false)
    this.renderTabList()
    this.overlay.classList.add('open')
    document.body.style.overflow = 'hidden'
    this.overlay.querySelector('#tab-label').focus()
  }

  close() {
    this.overlay.classList.remove('open')
    document.body.style.overflow = ''
    this.editingId = null
    this.removeCandidateId = null
    this.settingsTabId = null
    this.hideError()
    this.overlay.querySelector('#add-tab-form').reset()
    this.setFormMode(false)
  }

  renderTabList() {
    const list = this.overlay.querySelector('#tab-list')
    if (!list) return

    if (this.tabs.length === 0) {
      list.innerHTML = '<li class="empty">Nessun tab configurato</li>'
      return
    }

    list.innerHTML = this.tabs.map((tab, idx) => {
      const settingsOpen = this.settingsTabId === String(tab.id)
      const zoom = Math.round(tabZoom(tab) * 100)
      const isPanel = !!serviceForTab(tab)
      return `
      <li class="tab-item" data-tab-id="${tab.id}" draggable="true">
        <div class="tab-info">
          <span class="tab-drag-handle">${icon('chevronDown', 12)}</span>
          <span class="tab-icon">${icon(tab.icon, 16)}</span>
          <div>
            <span class="tab-title">${escapeHtml(tab.label)}</span>
            <span class="tab-url">${escapeHtml(tab.url || '—')}</span>
          </div>
        </div>
        <div class="tab-item-actions">
          ${this.removeCandidateId === String(tab.id) ? `
            <span class="remove-confirm">
              <span class="remove-confirm-text">Eliminare ${escapeHtml(tab.label)}?</span>
              <button class="btn btn-icon btn-danger remove-confirm-yes" data-tab-id="${tab.id}" aria-label="Conferma rimozione" title="Conferma">${icon('check', 14)}</button>
              <button class="btn btn-icon btn-icon-soft remove-confirm-no" data-tab-id="${tab.id}" aria-label="Annulla" title="Annulla">${icon('close', 14)}</button>
            </span>
          ` : `
            <button class="btn btn-icon btn-icon-soft tab-move-up" data-tab-id="${tab.id}" aria-label="Sposta su ${tab.label}" title="Sposta su" ${idx === 0 ? 'disabled' : ''}>${icon('chevronUp', 14)}</button>
            <button class="btn btn-icon btn-icon-soft tab-move-down" data-tab-id="${tab.id}" aria-label="Sposta giù ${tab.label}" title="Sposta giù" ${idx === this.tabs.length - 1 ? 'disabled' : ''}>${icon('chevronDown', 14)}</button>
            <button class="btn btn-icon btn-icon-soft tab-settings" data-tab-id="${tab.id}" aria-label="Visualizzazione ${tab.label}" title="Visualizzazione">${icon('sliders', 14)}</button>
            <button class="btn btn-icon btn-icon-soft edit-tab" data-tab-id="${tab.id}" aria-label="Modifica ${tab.label}" title="Modifica">${icon('edit', 14)}</button>
            <button class="btn btn-icon btn-danger remove-tab" data-tab-id="${tab.id}" aria-label="Rimuovi ${tab.label}" title="Rimuovi">${icon('close', 14)}</button>
          `}
        </div>
      </li>
      ${settingsOpen ? `
        <li class="tab-settings-panel" data-tab-id="${tab.id}">
          <div class="tab-settings-row">
            <label for="zoom-range-${tab.id}">Zoom</label>
            <div class="zoom-controls">
              <input type="range" id="zoom-range-${tab.id}" class="zoom-range" data-tab-id="${tab.id}" min="50" max="250" step="10" value="${zoom}" aria-label="Zoom ${tab.label}">
              <span class="zoom-value" id="zoom-value-${tab.id}">${zoom}%</span>
              <button class="btn btn-icon btn-icon-soft zoom-out" data-tab-id="${tab.id}" aria-label="Diminuisci zoom" title="Diminuisci zoom">${icon('minus', 14)}</button>
              <button class="btn btn-icon btn-icon-soft zoom-in" data-tab-id="${tab.id}" aria-label="Aumenta zoom" title="Aumenta zoom">${icon('plus', 14)}</button>
              <button class="btn btn-icon btn-icon-soft zoom-reset" data-tab-id="${tab.id}" aria-label="Reimposta zoom" title="Reimposta zoom">1:1</button>
            </div>
          </div>
          ${!isPanel ? `
            <label class="checkbox-row">
              <input type="checkbox" class="zoom-toolbar" data-tab-id="${tab.id}" ${tab.settings && tab.settings.toolbar ? 'checked' : ''}>
              <span>Mostra barra strumenti (ricarica e zoom)</span>
            </label>
          ` : ''}
        </li>
      ` : ''}
      `
    }).join('')

    list.querySelectorAll('.edit-tab').forEach(btn => {
      btn.addEventListener('click', (e) => this.startEdit(e.currentTarget.dataset.tabId))
    })
    list.querySelectorAll('.tab-settings').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const id = String(e.currentTarget.dataset.tabId)
        this.settingsTabId = (this.settingsTabId === id && this.editingId == null) ? null : id
        this.renderTabList()
      })
    })
    list.querySelectorAll('.zoom-out, .zoom-in').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.preventDefault()
        const id = String(e.currentTarget.dataset.tabId)
        const delta = e.currentTarget.classList.contains('zoom-in') ? 0.1 : -0.1
        const tab = this.tabs.find(t => String(t.id) === id)
        if (!tab) return
        this.applySettings(id, { zoom: Math.round((tabZoom(tab) + delta) * 10) / 10 })
      })
    })
    list.querySelectorAll('.zoom-reset').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.preventDefault()
        this.applySettings(String(e.currentTarget.dataset.tabId), { zoom: 1 })
      })
    })
    list.querySelectorAll('.zoom-range').forEach(range => {
      const id = String(range.dataset.tabId)
      const valueEl = this.overlay.querySelector('#zoom-value-' + id)
      range.addEventListener('input', () => {
        if (valueEl) valueEl.textContent = `${range.value}%`
      })
      range.addEventListener('change', () => {
        this.applySettings(id, { zoom: Number(range.value) / 100 })
      })
    })
    list.querySelectorAll('.zoom-toolbar').forEach(cb => {
      cb.addEventListener('change', (e) => {
        this.applySettings(String(e.currentTarget.dataset.tabId), { toolbar: e.currentTarget.checked })
      })
    })
    list.querySelectorAll('.remove-tab').forEach(btn => {
      btn.addEventListener('click', (e) => {
        this.removeCandidateId = String(e.currentTarget.dataset.tabId)
        this.renderTabList()
      })
    })
    list.querySelectorAll('.remove-confirm-yes').forEach(btn => {
      btn.addEventListener('click', (e) => {
        this.removeCandidateId = null
        this.confirmRemove(e.currentTarget.dataset.tabId)
      })
    })
    list.querySelectorAll('.remove-confirm-no').forEach(btn => {
      btn.addEventListener('click', (e) => {
        this.removeCandidateId = null
        this.renderTabList()
      })
    })
    list.querySelectorAll('.tab-move-up').forEach(btn => {
      btn.addEventListener('click', (e) => this.moveTab(e.currentTarget.dataset.tabId, -1))
    })
    list.querySelectorAll('.tab-move-down').forEach(btn => {
      btn.addEventListener('click', (e) => this.moveTab(e.currentTarget.dataset.tabId, 1))
    })
    list.querySelectorAll('.tab-item').forEach(item => {
      this.attachDragListeners(item)
    })
  }

  attachDragListeners(item) {
    const tabId = item.dataset.tabId

    item.addEventListener('dragstart', (e) => {
      this.draggingId = tabId
      item.classList.add('dragging')
      e.dataTransfer.effectAllowed = 'move'
      try { e.dataTransfer.setData('text/plain', String(tabId)) } catch { /* noop */ }
    })

    item.addEventListener('dragend', () => {
      item.classList.remove('dragging')
      this.overlay.querySelectorAll('.tab-item').forEach(el => el.classList.remove('drop-target'))
      this.draggingId = null
    })

    item.addEventListener('dragover', (e) => {
      if (this.draggingId == null) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'move'
      this.overlay.querySelectorAll('.tab-item').forEach(el => el.classList.remove('drop-target'))
      item.classList.add('drop-target')
    })

    item.addEventListener('drop', (e) => {
      e.preventDefault()
      const fromId = this.draggingId
      const toId = tabId
      item.classList.remove('drop-target')
      if (fromId == null || String(fromId) === String(toId)) return

      const ids = this.tabs.map(t => t.id)
      const fromIdx = ids.findIndex(id => String(id) === String(fromId))
      const toIdx = ids.findIndex(id => String(id) === String(toId))
      if (fromIdx === -1 || toIdx === -1) return

      const [moved] = ids.splice(fromIdx, 1)
      ids.splice(toIdx, 0, moved)
      this.tabs = ids.map(id => this.tabs.find(t => String(t.id) === String(id)))
      this.renderTabList()
      if (this.onReorder) this.onReorder(ids)
    })
  }

  moveTab(tabId, direction) {
    const idx = this.tabs.findIndex(t => String(t.id) === String(tabId))
    const target = idx + direction
    if (idx === -1 || target < 0 || target >= this.tabs.length) return
    const [moved] = this.tabs.splice(idx, 1)
    this.tabs.splice(target, 0, moved)
    this.renderTabList()
    if (this.onReorder) this.onReorder(this.tabs.map(t => t.id))
  }

  // Applies a per-tab display settings patch optimistically, re-renders the
  // list (keeping the panel open) and notifies app.js to persist + apply.
  async applySettings(tabId, patch) {
    const tab = this.tabs.find(t => String(t.id) === String(tabId))
    if (!tab) return
    const merged = { ...(tab.settings && typeof tab.settings === 'object' ? tab.settings : {}), ...patch }
    tab.settings = merged
    this.renderTabList()
    if (this.onUpdateSettings) {
      try {
        await this.onUpdateSettings(tabId, merged)
      } catch (error) {
        this.showError('Errore: ' + error.message)
      }
    }
  }

  startEdit(tabId) {
    const tab = this.tabs.find(t => String(t.id) === String(tabId))
    if (!tab) return
    this.editingId = tabId
    this.settingsTabId = null
    const form = this.overlay.querySelector('#add-tab-form')
    form.elements.label.value = tab.label || ''
    form.elements.icon.value = this.iconAvailable(tab.icon) ? tab.icon : 'server'
    form.elements.url.value = tab.url || ''
    this.setFormMode(true)
    this.overlay.querySelector('#tab-label').focus()
  }

  cancelEdit() {
    this.editingId = null
    const form = this.overlay.querySelector('#add-tab-form')
    form.reset()
    form.elements.icon.value = 'brain'
    this.setFormMode(false)
  }

  setFormMode(editing) {
    const title = this.overlay.querySelector('#form-section-title')
    title.textContent = editing ? 'Modifica Tab' : 'Aggiungi Nuovo Tab'
    const submitBtn = this.overlay.querySelector('#submit-tab-btn')
    submitBtn.innerHTML = editing ? `${icon('check', 14)} Salva Modifiche` : `${icon('plus', 14)} Aggiungi`
    this.overlay.querySelector('#cancel-edit-btn').style.display = editing ? '' : 'none'
  }

  iconAvailable(name) {
    return ICONS.map(([value]) => value).includes(name)
  }

  showError(message) {
    const box = this.overlay.querySelector('#modal-error')
    if (!box) return
    box.textContent = message
    box.hidden = false
  }

  hideError() {
    const box = this.overlay.querySelector('#modal-error')
    if (box) box.hidden = true
  }

  async handleSubmit(e) {
    e.preventDefault()
    this.hideError()
    const form = e.target
    const formData = new FormData(form)

    const config = {
      label: (formData.get('label') || '').trim(),
      icon: formData.get('icon') || 'server',
      url: (formData.get('url') || '').trim(),
    }

    if (!config.label) {
      this.showError('Inserisci un etichetta per il tab.')
      return
    }

    try {
      if (this.editingId != null) {
        await this.onUpdateTab(this.editingId, config)
        this.tabs = this.tabs.map(t =>
          String(t.id) === String(this.editingId)
            ? { ...t, label: config.label, icon: config.icon, url: config.url }
            : t
        )
        this.renderTabList()
        this.cancelEdit()
      } else {
        await this.onSave(config)
        form.reset()
        form.elements.icon.value = 'brain'
      }
    } catch (error) {
      this.showError('Errore: ' + error.message)
    }
  }

  async confirmRemove(tabId) {
    try {
      await this.onRemoveTab(tabId)
      this.tabs = this.tabs.filter(t => String(t.id) !== String(tabId))
      this.renderTabList()
    } catch (error) {
      this.showError('Errore: ' + error.message)
    }
  }
}