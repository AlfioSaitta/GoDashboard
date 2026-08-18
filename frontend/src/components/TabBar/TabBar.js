import { createElement, icon, escapeHtml } from '../Shared/utils.js'
import { serviceForTab, statusForTab, urlForTab, tabZoom } from '../Shared/services.js'

const ZOOM_STEP = 0.1

export class TabBar {
  constructor(container, {
    onTabChange, onAddTab, onReorder, onSetDefault,
    onOpenExternal, onRenameTab, onDuplicateTab, onNotes,
    onZoom, onResetZoom, onPopupChange, onTerminal, onNav,
  }) {
    this.container = container
    this.onTabChange = onTabChange
    this.onAddTab = onAddTab
    this.onReorder = onReorder
    this.onSetDefault = onSetDefault
    this.onOpenExternal = onOpenExternal
    this.onRenameTab = onRenameTab
    this.onDuplicateTab = onDuplicateTab
    this.onNotes = onNotes
    this.onZoom = onZoom
    this.onResetZoom = onResetZoom
    this.onPopupChange = onPopupChange
    this.onTerminal = onTerminal
    this.onNav = onNav
    this.tabs = []
    this.statuses = []
    this.activeTab = null
    this.draggingId = null
    this.navState = {}        // tabId -> {canGoBack, canGoForward, loading}
    this.terminalState = {}   // tabId -> {running, visible}
    this.render()
  }

  render() {
    this.container.innerHTML = `
      <div class="tab-bar-inner">
        <div class="tab-nav-controls" role="group" aria-label="Navigazione pagina">
          <button type="button" class="nav-btn" id="nav-back" title="Indietro" aria-label="Indietro" disabled>${icon('chevronLeft', 15)}</button>
          <button type="button" class="nav-btn" id="nav-forward" title="Avanti" aria-label="Avanti" disabled>${icon('chevronRight', 15)}</button>
          <button type="button" class="nav-btn nav-reload" id="nav-reload" title="Ricarica" aria-label="Ricarica" disabled>${icon('reload', 14)}</button>
          <span class="nav-divider"></span>
          <button type="button" class="nav-btn" id="nav-zoom-out" title="Zoom indietro" aria-label="Zoom indietro" disabled>${icon('zoomOut', 14)}</button>
          <button type="button" class="nav-btn nav-zoom-label" id="nav-zoom" title="Reimposta zoom (100%)">100%</button>
          <button type="button" class="nav-btn" id="nav-zoom-in" title="Zoom avanti" aria-label="Zoom avanti" disabled>${icon('zoomIn', 14)}</button>
        </div>
        <div class="tabs-container" id="tabs-container"></div>
        <div class="tab-actions">
          <button class="btn btn-icon btn-add-tab" id="add-tab-btn" aria-label="Aggiungi tab">${icon('plus', 16)}</button>
        </div>
      </div>
      <div class="tab-context-menu" id="tab-context-menu" hidden></div>
    `
    this.container.querySelector('#add-tab-btn').addEventListener('click', this.onAddTab)

    // ── Page navigation controls (act on the ACTIVE tab) ──
    this.container.querySelector('#nav-back').addEventListener('click', () => this.nav('back'))
    this.container.querySelector('#nav-forward').addEventListener('click', () => this.nav('forward'))
    this.container.querySelector('#nav-reload').addEventListener('click', () => this.nav('reload'))
    this.container.querySelector('#nav-zoom').addEventListener('click', () => {
      const tab = this.active()
      if (!tab) return
      if (this.onResetZoom) this.onResetZoom(tab)
    })
    this.container.querySelector('#nav-zoom-out').addEventListener('click', () => {
      const tab = this.active()
      if (tab && this.onZoom) this.onZoom(tab, -ZOOM_STEP)
    })
    this.container.querySelector('#nav-zoom-in').addEventListener('click', () => {
      const tab = this.active()
      if (tab && this.onZoom) this.onZoom(tab, ZOOM_STEP)
    })
    this.renderTabs()
  }

  active() {
    return this.tabs.find(t => String(t.id) === String(this.activeTab)) || null
  }

  nav(action) {
    if (!this.onNav) return
    const tab = this.active()
    if (!tab) return
    // While a page is loading the reload button means "stop".
    if (action === 'reload' && this.navState[String(tab.id)]?.loading) action = 'stop'
    this.onNav(tab.id, action)
  }

  // Navigation state of a tab page (from the shell:nav-state event).
  setNavState(tabId, state) {
    this.navState[String(tabId)] = state || {}
    this.refreshNavControls()
  }

  // Terminal state of a tab (from the shell:terminal-state event).
  setTerminalState(tabId, state) {
    this.terminalState[String(tabId)] = { running: false, visible: false, ...(state || {}) }
    this.refreshTerminalButton(tabId)
  }

  refreshTerminalButton(tabId) {
    const item = this.container.querySelector(`.tab-bar-item[data-tab-id="${CSS.escape(String(tabId))}"]`)
    if (!item) return
    const btn = item.querySelector('.tab-terminal-btn')
    if (!btn) return
    const st = this.terminalState[String(tabId)]
    btn.classList.toggle('open', !!(st && st.visible))
    btn.title = (st && st.visible) ? 'Terminale aperto — chiudi' : 'Terminale SSH'
  }

  refreshNavControls() {
    const tab = this.active()
    const btn = (id) => this.container.querySelector(id)
    const back = btn('#nav-back')
    const forward = btn('#nav-forward')
    const reload = btn('#nav-reload')
    const zoomOut = btn('#nav-zoom-out')
    const zoomIn = btn('#nav-zoom-in')
    const zoom = btn('#nav-zoom')

    const disabled = !tab
    back.disabled = disabled || !(this.navState[tab ? String(tab.id) : ''] && this.navState[String(tab.id)].canGoBack)
    forward.disabled = disabled || !(this.navState[tab ? String(tab.id) : ''] && this.navState[String(tab.id)].canGoForward)
    reload.disabled = disabled
    zoomOut.disabled = disabled
    zoomIn.disabled = disabled
    zoom.disabled = disabled

    const loading = !disabled && !!this.navState[String(tab.id)]?.loading
    reload.classList.toggle('nav-loading', loading)
    if (loading) {
      reload.dataset.mode = 'stop'
      reload.title = 'Interrompi'
      reload.innerHTML = icon('stop', 13)
    } else {
      delete reload.dataset.mode
      reload.title = 'Ricarica'
      reload.innerHTML = icon('reload', 14)
    }

    if (zoom) zoom.textContent = disabled ? '100%' : `${Math.round(tabZoom(tab) * 100)}%`
  }

  setStatuses(statuses) {
    this.statuses = statuses || []
    this.applyStatusBadges()
  }

  statusFor(tab) {
    return statusForTab(tab, this.statuses)
  }

  tabTooltip(tab) {
    const url = urlForTab(tab)
    if (url && url !== String(tab.url || '').trim()) {
      return `${tab.label}\n${url}`
    }
    if (tab.url && !serviceForTab(tab)) {
      return `${tab.label}\n${tab.url}`
    }
    return tab.label
  }

  renderTabs() {
    const container = this.container.querySelector('#tabs-container')
    container.innerHTML = ''

    for (const tab of this.tabs) {
      const hasNotes = !!(tab.notes && tab.notes.length > 0)
      const termState = this.terminalState[String(tab.id)]
      const termOpen = !!(termState && termState.visible)
      const item = createElement(`
        <div class="tab-bar-item" data-tab-id="${tab.id}" draggable="true"
             title="${escapeHtml(this.tabTooltip(tab))}">
          <span class="tab-status-dot"></span>
          <button class="tab-btn" title="${escapeHtml(this.tabTooltip(tab))}">
            ${icon(tab.icon, 14)}
            <span class="tab-label">${escapeHtml(tab.label)}</span>
          </button>
          <button class="tab-terminal-btn ${termOpen ? 'open' : ''}" data-tab-id="${tab.id}" title="${termOpen ? 'Terminale aperto — chiudi' : 'Terminale SSH'}">
            ${icon('terminal', 12)}
          </button>
          <button class="tab-note-btn ${hasNotes ? 'has-note' : ''}" data-tab-id="${tab.id}"
                  title="${hasNotes ? 'Note presenti — modifica' : 'Aggiungi una nota'}">
            ${icon('note', 12)}
          </button>
        </div>
      `)

      item.querySelector('.tab-btn').addEventListener('click', () => {
        this.onTabChange(tab.id)
      })

      item.querySelector('.tab-terminal-btn').addEventListener('click', (e) => {
        e.stopPropagation()
        if (this.onTerminal) this.onTerminal(tab)
      })

      item.querySelector('.tab-note-btn').addEventListener('click', (e) => {
        e.stopPropagation()
        if (this.onNotes) this.onNotes(tab)
      })

      item.addEventListener('contextmenu', (e) => {
        e.preventDefault()
        e.stopPropagation()
        this.showContextMenu(e.clientX, e.clientY, tab)
      })

      this.attachDragListeners(item, tab.id)

      container.appendChild(item)
    }

    this.setActive(this.activeTab)
    this.applyStatusBadges()
    this.refreshNavControls()
  }

  attachDragListeners(item, tabId) {
    item.addEventListener('dragstart', (e) => {
      this.draggingId = tabId
      item.classList.add('dragging')
      e.dataTransfer.effectAllowed = 'move'
      try { e.dataTransfer.setData('text/plain', String(tabId)) } catch { /* noop */ }
    })

    item.addEventListener('dragend', () => {
      item.classList.remove('dragging')
      this.container.querySelectorAll('.tab-bar-item').forEach(el => el.classList.remove('drop-target'))
      this.draggingId = null
    })

    item.addEventListener('dragover', (e) => {
      if (this.draggingId == null) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'move'
      this.container.querySelectorAll('.tab-bar-item').forEach(el => el.classList.remove('drop-target'))
      item.classList.add('drop-target')
    })

    item.addEventListener('dragleave', () => {
      item.classList.remove('drop-target')
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

      const reordered = ids.map(id => this.tabs.find(t => String(t.id) === String(id)))
      this.tabs = reordered
      this.renderTabs()
      if (this.onReorder) this.onReorder(ids)
    })
  }

  showContextMenu(x, y, tab) {
    const menu = this.container.querySelector('#tab-context-menu')
    const isDefault = this.container.dataset.defaultTabId === String(tab.id)
    const zoom = Math.round(tabZoom(tab) * 100)

    const items = [
      { label: 'Rinomina', icon: 'edit', action: () => this.startInlineRename(tab) },
      { label: isDefault ? '✓ Tab predefinito' : 'Imposta come predefinito', icon: 'check', action: () => this.onSetDefault && this.onSetDefault(tab.id) },
      { label: 'Apri in browser', icon: 'external', action: () => this.onOpenExternal && this.onOpenExternal(tab) },
      { label: 'Duplica', icon: 'copy', action: () => this.onDuplicateTab && this.onDuplicateTab(tab) },
      { label: 'Terminale SSH', icon: 'terminal', action: () => this.onTerminal && this.onTerminal(tab) },
      { label: 'Nota', icon: 'note', action: () => this.onNotes && this.onNotes(tab) },
      { divider: true },
      { label: 'Zoom −', icon: 'minus', keep: true, action: () => this.onZoom && this.onZoom(tab, -ZOOM_STEP) },
      { label: `${zoom}%`, icon: 'chart', disabled: true },
      { label: 'Zoom +', icon: 'plus', keep: true, action: () => this.onZoom && this.onZoom(tab, ZOOM_STEP) },
      { label: zoom === 100 ? 'Zoom 100%' : 'Reimposta zoom', icon: 'refresh', keep: true, action: () => this.onResetZoom && this.onResetZoom(tab) },
    ]

    menu.innerHTML = items.map((it, i) => it.divider
      ? '<div class="ctx-divider"></div>'
      : it.disabled
        ? `<span class="ctx-label" data-idx="${i}">${icon(it.icon, 14)} ${it.label}</span>`
        : `<button class="ctx-item" data-idx="${i}">${icon(it.icon, 14)} ${it.label}</button>`
    ).join('')

    const close = () => {
      menu.hidden = true
      document.removeEventListener('click', onDocClick)
      document.removeEventListener('keydown', onKeyDown)
      if (this.onPopupChange) this.onPopupChange(false)
    }
    const onDocClick = (e) => {
      if (menu.contains(e.target)) return
      close()
    }
    const onKeyDown = (e) => {
      if (e.key === 'Escape') close()
    }

    menu.querySelectorAll('.ctx-item').forEach(btn => {
      btn.addEventListener('click', () => {
        const it = items[Number(btn.dataset.idx)]
        if (!it) return
        if (it.keep) {
          // Keep the menu open so zoom can be adjusted with repeated clicks;
          // rebuild in place (the percentage label reflects the new zoom).
          close()
          if (it.action) it.action()
          this.showContextMenu(x, y, tab)
          return
        }
        close()
        if (it.action) it.action()
      })
    })
    document.addEventListener('click', onDocClick)
    document.addEventListener('keydown', onKeyDown)

    menu.hidden = false
    if (this.onPopupChange) this.onPopupChange(true)
    const rect = menu.getBoundingClientRect()
    // Keep the open menu inside the (temporarily expanded) window.
    const left = Math.min(x, window.innerWidth - rect.width - 8)
    const top = Math.min(y, window.innerHeight - rect.height - 8)
    menu.style.left = `${Math.max(8, left)}px`
    menu.style.top = `${Math.max(8, top)}px`
  }

  startInlineRename(tab) {
    const item = this.container.querySelector(`.tab-bar-item[data-tab-id="${tab.id}"]`)
    if (!item) return
    const label = item.querySelector('.tab-label')
    if (!label) return

    const input = document.createElement('input')
    input.type = 'text'
    input.className = 'tab-rename-input'
    input.value = tab.label
    input.maxLength = 60
    label.replaceWith(input)
    item.classList.add('renaming')
    input.focus()
    input.select()

    let done = false
    const finish = (commit) => {
      if (done) return
      done = true
      item.classList.remove('renaming')
      const value = input.value.trim()
      input.removeEventListener('keydown', onKey)
      input.removeEventListener('blur', onBlur)
      if (commit && value && value !== tab.label && this.onRenameTab) {
        this.onRenameTab(tab.id, value)
        return
      }
      const fresh = this.tabs.find(t => String(t.id) === String(tab.id)) || tab
      input.replaceWith(this.makeLabelElement(fresh))
      this.setActive(this.activeTab)
    }
    const onKey = (e) => {
      if (e.key === 'Enter') { e.preventDefault(); finish(true) }
      else if (e.key === 'Escape') { e.preventDefault(); finish(false) }
    }
    const onBlur = () => finish(true)
    input.addEventListener('keydown', onKey)
    setTimeout(() => input.addEventListener('blur', onBlur), 0)
  }

  makeLabelElement(tab) {
    const span = document.createElement('span')
    span.className = 'tab-label'
    span.textContent = tab.label
    return span
  }

  setTabs(tabs, { persist = false } = {}) {
    this.tabs = tabs || []
    this.renderTabs()
    if (persist && this.onReorder) {
      this.onReorder(this.tabs.map(t => t.id))
    }
  }

  setActive(tabId) {
    this.activeTab = tabId
    const strId = String(tabId)
    this.container.querySelectorAll('.tab-bar-item').forEach(item => {
      item.classList.toggle('active', String(item.dataset.tabId) === strId)
    })
    // Scroll the active tab into view when the tab bar overflows.
    const active = this.container.querySelector(`.tab-bar-item.active`)
    if (active) active.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' })
    this.refreshNavControls()
  }

  // Sets a TRANSIENT page title on a tab (the rendered <title> of its native
  // webview). Only the display text changes: the stored label stays intact and
  // is restored on the next renderTabs().
  setPageTitle(tabId, title) {
    if (!title || !String(title).trim()) return
    const item = this.container.querySelector(`.tab-bar-item[data-tab-id="${CSS.escape(String(tabId))}"]`)
    if (!item) return
    const label = item.querySelector('.tab-label')
    if (label && !item.classList.contains('renaming')) label.textContent = String(title)
  }

  setDefaultTabId(tabId) {
    this.container.dataset.defaultTabId = String(tabId)
  }

  applyStatusBadges() {
    this.container.querySelectorAll('.tab-bar-item').forEach(item => {
      const tab = this.tabs.find(t => String(t.id) === String(item.dataset.tabId))
      if (!tab) return
      const status = this.statusFor(tab)
      const dot = item.querySelector('.tab-status-dot')
      if (dot) {
        dot.classList.remove('online', 'offline')
        if (status) dot.classList.add(status.healthy ? 'online' : 'offline')
      }
    })
  }
}