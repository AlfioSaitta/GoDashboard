import { TabBar } from '@/components/TabBar/TabBar.js'
import { NeuroNetPanel } from '@/components/NeuroNetPanel/NeuroNetPanel.js'
import { MinecraftPanel } from '@/components/MinecraftPanel/MinecraftPanel.js'
import { SlotBuilderPanel } from '@/components/SlotBuilderPanel/SlotBuilderPanel.js'
import { SettingsModal } from '@/components/SettingsModal/SettingsModal.js'
import { dashboardStore } from '@/stores/dashboard.js'
import { api } from '@/services/api.js'
import { createElement, icon } from '@/components/Shared/utils.js'
import { serviceForTab, urlForTab as resolveUrlForTab } from '@/components/Shared/services.js'

export async function createApp() {
  try {
    await applyTheme()
  } catch { /* keep default */ }

  const app = document.getElementById('app')
  app.innerHTML = `
    <div class="dashboard">
      <header class="dashboard-header">
        <div class="header-left">
          <div class="brand-mark">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="7" rx="1.5"></rect>
              <rect x="14" y="3" width="7" height="7" rx="1.5"></rect>
              <rect x="3" y="14" width="7" height="7" rx="1.5"></rect>
              <path d="M17.5 14a2.5 2.5 0 0 1 0 5 2.5 2.5 0 0 1 0-5Z"></path>
            </svg>
          </div>
          <div class="header-text">
            <h1 class="dashboard-title">Dashboard</h1>
          </div>
        </div>
        <div class="header-right">
          <div class="inspector-wrap" data-win="no-drag">
            <button id="inspector-btn" class="btn btn-icon btn-settings" aria-label="Ispettore" title="Ispettore">
              ${icon('code', 18)}
            </button>
            <div id="inspector-menu" class="inspector-menu" role="menu" hidden></div>
          </div>
          <button data-win="no-drag" id="settings-btn" class="btn btn-icon btn-settings" aria-label="Impostazioni" title="Impostazioni">
            ${icon('settings', 18)}
          </button>
          <div class="win-controls" role="group" aria-label="Controlli finestra">
            <button data-win="no-drag" class="win-btn" id="win-min" aria-label="Minimizza" title="Minimizza">
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2 6h8"></path>
              </svg>
            </button>
            <button data-win="no-drag" class="win-btn" id="win-max" aria-label="Massimizza" title="Massimizza">
              <svg class="icon-max" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5">
                <rect x="2" y="2" width="8" height="8" rx="1"></rect>
              </svg>
              <svg class="icon-restore" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" style="display:none">
                <rect x="2.5" y="4" width="5.5" height="5.5" rx="1"></rect>
                <path d="M4.5 2.5h4a1 1 0 0 1 1 1v4"></path>
              </svg>
            </button>
            <button data-win="no-drag" class="win-btn win-btn-close" id="win-close" aria-label="Chiudi" title="Chiudi">
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2.5 2.5l7 7"></path>
                <path d="M9.5 2.5l-7 7"></path>
              </svg>
            </button>
          </div>
        </div>
      </header>
      <nav class="tab-bar" id="tab-bar"></nav>
      <main class="dashboard-content" id="dashboard-content"></main>
    </div>
    <div id="settings-modal" class="modal-overlay" role="dialog" aria-modal="true" aria-labelledby="settings-title"></div>
  `

  const contentEl = document.getElementById('dashboard-content')
  let activePanelTabId = null

  // ── Per-tab display settings (zoom, toolbar) ────────────
  const ZOOM_MIN = 0.5
  const ZOOM_MAX = 2.5
  const ZOOM_STEP = 0.1

  function clampZoom(z) {
    return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(z * 10) / 10))
  }

  function zoomOf(tab) {
    const z = tab && tab.settings ? Number(tab.settings.zoom) : NaN
    return Number.isFinite(z) ? clampZoom(z) : 1
  }

  // Applies the stored per-tab settings to an existing (kept-alive) view.
  function applyViewSettings(view, tab) {
    if (!view || !view.el) return
    view.el.style.zoom = String(zoomOf(tab))
    view.el.classList.toggle('has-toolbar', !view.panel && !!(tab.settings && tab.settings.toolbar))
    if (view.toolbarRef) view.toolbarRef.updateZoom(zoomOf(tab))
  }

  // Merges a settings patch into a tab: updates the store, applies it right
  // away on the live view and persists it on the backend.
  async function updateTabSettings(tab, settings) {
    const sel = dashboardStore.getTabs().find(t => String(t.id) === String(tab.id))
    if (sel) sel.settings = settings
    tab.settings = settings
    const view = viewCache.get(String(tab.id))
    if (view) applyViewSettings(view, tab)
    try {
      await api.updateTabSettings(tab.id, settings)
    } catch (error) {
      console.error('Failed to save tab settings:', error)
    }
  }

  function setTabZoom(tab, z) {
    const current = (tab.settings && typeof tab.settings === 'object') ? tab.settings : {}
    return updateTabSettings(tab, { ...current, zoom: clampZoom(z) })
  }

  const tabBar = new TabBar(document.getElementById('tab-bar'), {
    onTabChange: switchTab,
    onAddTab: openSettings,
    onReorder: async (ids) => {
      try {
        await api.reorderTabs(ids)
        await loadTabs({ preserveActive: true })
      } catch (error) {
        console.error('Failed to reorder tabs:', error)
      }
    },
    onSetDefault: (tabId) => {
      dashboardStore.setDefaultTab(tabId)
      tabBar.setDefaultTabId(tabId)
    },
    onOpenExternal: (tab) => {
      const url = urlForTab(tab)
      if (url) api.openExternal(url).catch(err => console.error('Failed to open URL:', err))
    },
    onRenameTab: async (tabId, label) => {
      const tab = dashboardStore.getTabs().find(t => String(t.id) === String(tabId))
      try {
        await api.updateTab(tabId, { label, url: tab ? tab.url : '', icon: tab ? tab.icon : 'server' })
        await loadTabs({ preserveActive: true })
      } catch (error) {
        await loadTabs({ preserveActive: true })
        console.error('Failed to rename tab:', error)
      }
    },
    onDuplicateTab: async (tab) => {
      try {
        await api.saveTabConfig({ label: `${tab.label} (copia)`, icon: tab.icon || 'server', url: tab.url || '' })
        await loadTabs({ preserveActive: true })
      } catch (error) {
        console.error('Failed to duplicate tab:', error)
      }
    },
    onZoom: (tab, delta) => setTabZoom(tab, zoomOf(tab) + delta),
    onResetZoom: (tab) => setTabZoom(tab, 1),
    onToggleToolbar: async (tab) => {
      const current = (tab.settings && typeof tab.settings === 'object') ? tab.settings : {}
      await updateTabSettings(tab, { ...current, toolbar: !current.toolbar })
    },
  })

  const panels = {
    neuronet: new NeuroNetPanel(),
    minecraft: new MinecraftPanel(),
    slotbuilder: new SlotBuilderPanel(),
  }

  const settingsModal = new SettingsModal(document.getElementById('settings-modal'), {
    onSave: async (config) => {
      await api.saveTabConfig(config)
      await loadTabs()
    },
    onUpdateTab: async (tabId, config) => {
      await api.updateTab(tabId, config)
      await loadTabs()
    },
    onRemoveTab: async (tabId) => {
      await api.removeTab(tabId)
      await loadTabs()
    },
    onReorder: async (ids) => {
      try {
        await api.reorderTabs(ids)
        await loadTabs({ preserveActive: true })
      } catch (error) {
        console.error('Failed to reorder tabs:', error)
      }
    },
    onThemeChange: async (theme) => {
      try {
        await api.setTheme(theme)
        await applyThemeUI(theme)
      } catch (error) {
        console.error('Failed to save theme:', error)
      }
    },
    onUpdateSettings: async (tabId, patch) => {
      const tab = dashboardStore.getTabs().find(t => String(t.id) === String(tabId))
      if (!tab) return
      const current = (tab.settings && typeof tab.settings === 'object') ? tab.settings : {}
      await updateTabSettings(tab, { ...current, ...patch })
    },
    onClose: closeSettings,
  })

  let serviceStatusBusy = false
  async function loadServiceStatus() {
    if (serviceStatusBusy) return
    serviceStatusBusy = true
    try {
      const statuses = await api.getServicesStatus()
      const list = statuses || []
      dashboardStore.lastStatuses = list
      tabBar.setStatuses(list)
    } catch { /* keep last known */ } finally {
      serviceStatusBusy = false
    }
  }

  function scheduleStatusPoll() {
    const poll = () => {
      if (!document.hidden) loadServiceStatus()
    }
    setInterval(poll, 30000)
    document.addEventListener('visibilitychange', poll)
  }

  async function loadTabs({ preserveActive = false } = {}) {
    try {
      const tabs = await api.getTabs()
      const list = tabs || []
      const previousActive = activePanelTabId

      dashboardStore.setTabs(list)
      tabBar.setStatuses(dashboardStore.lastStatuses || [])
      tabBar.setTabs(list)
      tabBar.setDefaultTabId(dashboardStore.getDefaultTab())

      // Drop cached views for tabs that no longer exist, so removed tabs free
      // their iframes/sessions instead of lingering in the DOM.
      const alive = new Set(list.map(t => String(t.id)))
      viewCache.forEach((view, tabId) => {
        if (!alive.has(String(tabId))) destroyView(view, tabId)
      })

      if (list.length > 0) {
        let target
        if (preserveActive && previousActive != null &&
            alive.has(String(previousActive))) {
          target = previousActive
        } else {
          const defaultTabId = dashboardStore.getDefaultTab()
          target = list.find(t => String(t.id) === String(defaultTabId)) ? defaultTabId : list[0].id
        }
        switchTab(target)
      } else {
        showEmptyState()
      }
    } catch (error) {
      console.error('Failed to load tabs:', error)
      showEmptyState(error.message)
    }
  }

function panelForTab(tab) {
  if (!tab) return null
  const svc = serviceForTab(tab)
  if (!svc) return null
  return panels[svc.id] || null
}

function urlForTab(tab) {
  return resolveUrlForTab(tab)
}

  // Tab views are kept alive once created. Switching only toggles visibility,
  // so iframes keep their browsing context (login/session/cookies) intact.
  const viewCache = new Map() // tabId -> { el, panel }
  let currentViewId = null

  function switchTab(tabId) {
    const tab = dashboardStore.getTabs().find(t => String(t.id) === String(tabId))

    if (currentViewId != null && viewCache.has(String(currentViewId))) {
      const prev = viewCache.get(String(currentViewId))
      prev.el.classList.remove('active')
      if (prev.panel) prev.panel.stopAutoRefresh()
    }

    if (tab) {
      const view = ensureView(tab)
      applyViewSettings(view, tab)
      view.el.classList.add('active')
      if (view.panel) view.panel.startAutoRefresh()
      currentViewId = tab.id
      activePanelTabId = tab.id
    } else {
      currentViewId = null
      activePanelTabId = null
      hideAllViews()
      contentEl.innerHTML = '<div class="empty-state">Tab non disponibile</div>'
    }

    tabBar.setActive(tab ? tab.id : null)
  }

  function hideAllViews() {
    viewCache.forEach(v => {
      v.el.classList.remove('active')
      if (v.panel) v.panel.stopAutoRefresh()
    })
  }

  function viewKey(tab) {
    const panel = panelForTab(tab)
    return panel ? `panel` : `url:${tab.url}`
  }

  function ensureView(tab) {
    let view = viewCache.get(String(tab.id))
    if (view && view.key === viewKey(tab)) {
      return view
    }
    if (view) destroyView(view, String(tab.id))

    view = { el: null, panel: null, key: viewKey(tab) }
    const panel = panelForTab(tab)
    if (panel) {
      if (!panel.element) panel.render()
      view.el = panel.element
      view.panel = panel
      if (!panel.mounted) {
        panel.mounted = true
        panel.mount(contentEl)
        panel.refresh()
      }
    } else {
      const settings = (tab.settings && typeof tab.settings === 'object') ? tab.settings : {}
      view.el = createElement(`
        <div class="panel url-panel${settings.toolbar ? ' has-toolbar' : ''}">
          <div class="url-toolbar">
            <button class="btn btn-icon url-reload" title="Ricarica" aria-label="Ricarica">${icon('refresh', 14)}</button>
            <span class="url-toolbar-spacer"></span>
            <button class="btn btn-icon url-zoom-out" title="Diminuisci zoom" aria-label="Diminuisci zoom">${icon('minus', 14)}</button>
            <span class="url-zoom-label">100%</span>
            <button class="btn btn-icon url-zoom-in" title="Aumenta zoom" aria-label="Aumenta zoom">${icon('plus', 14)}</button>
            <button class="btn btn-icon url-zoom-reset" title="Reimposta zoom" aria-label="Reimposta zoom">${icon('reset', 14)}</button>
          </div>
          <div class="panel-content">
            <iframe class="tab-frame" src="${tab.url}" title="${tab.label}"></iframe>
          </div>
        </div>
      `)
      const frame = view.el.querySelector('.tab-frame')
      view.el.querySelector('.url-reload').addEventListener('click', () => { frame.src = frame.src })
      const zoomLabel = view.el.querySelector('.url-zoom-label')
      view.el.querySelector('.url-zoom-out').addEventListener('click', () => setTabZoom(tab, zoomOf(tab) - ZOOM_STEP))
      view.el.querySelector('.url-zoom-in').addEventListener('click', () => setTabZoom(tab, zoomOf(tab) + ZOOM_STEP))
      view.el.querySelector('.url-zoom-reset').addEventListener('click', () => setTabZoom(tab, 1))
      view.toolbarRef = {
        updateZoom: (z) => { zoomLabel.textContent = `${Math.round(z * 100)}%` },
      }
    }

    contentEl.appendChild(view.el)
    viewCache.set(String(tab.id), view)
    return view
  }

  function destroyView(view, tabId) {
    viewCache.delete(String(tabId))
    if (view.panel) view.panel.stopAutoRefresh()
    if (view.el && view.el.parentNode === contentEl) {
      contentEl.removeChild(view.el)
    }
  }

  function showEmptyState(message) {
    hideAllViews()
    contentEl.innerHTML = ''
    const empty = document.createElement('div')
    empty.className = 'empty-state'
    empty.innerHTML = `
      <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <rect x="2" y="3" width="20" height="14" rx="2"></rect>
        <path d="M8 21h8"></path>
        <path d="M12 17v4"></path>
      </svg>
      <h2>Nessun tab configurato</h2>
      <p>${message || 'Apri le impostazioni per aggiungere i tuoi servizi'}</p>
      <button class="btn btn-primary" id="empty-settings-btn">Apri Impostazioni</button>
    `
    contentEl.appendChild(empty)
    document.getElementById('empty-settings-btn')?.addEventListener('click', openSettings)
    tabBar.setActive(null)
  }

  function closeSettings() {
    settingsModal.close()
  }

  document.getElementById('settings-btn').addEventListener('click', openSettings)

  // ── Inspector dropdown ──────────────────────────────────
  const INSPECTOR_MODES = [
    { mode: 'bottom', label: 'Aggancia in basso', icon: 'chevronDown' },
    { mode: 'right', label: 'Aggancia a destra', icon: 'layers' },
    { mode: 'left', label: 'Aggancia a sinistra', icon: 'layers' },
    { mode: 'float', label: 'Finestra fluttuante', icon: 'external' },
  ]
  const inspectorBtn = document.getElementById('inspector-btn')
  const inspectorMenu = document.getElementById('inspector-menu')

  function renderInspectorMenu() {
    inspectorMenu.innerHTML = [
      ...INSPECTOR_MODES.map(({ mode, label, icon: icn }) =>
        `<button class="ctx-item" data-mode="${mode}">${icon(icn, 14)} ${label}</button>`),
      `<div class="ctx-divider"></div>`,
      `<button class="ctx-item" data-mode="close">${icon('close', 14)} Chiudi ispettore</button>`,
    ].join('')
    inspectorMenu.querySelectorAll('.ctx-item').forEach(btn => {
      btn.addEventListener('click', async () => {
        const mode = btn.dataset.mode
        // Inspect the PAGE of the active tab (service admin URL for built-in
        // panels, the iframe URL for URL tabs) via a dedicated webview.
        const activeTab = dashboardStore.getTabs()
          .find(t => String(t.id) === String(activePanelTabId))
        const url = urlForTab(activeTab) || ''
        try {
          if (mode === 'close') await api.inspectorClose()
          else {
            if (!url) {
              console.warn('Nessun URL ispezionabile per il tab attivo:', activeTab && activeTab.label)
            } else {
              await api.inspectorOpen(mode, url)
            }
          }
        } catch (error) {
          console.error('Inspector action failed:', error)
        }
        hideInspectorMenu()
      })
    })
  }

  function hideInspectorMenu() {
    inspectorMenu.hidden = true
    document.removeEventListener('click', onInspectorDocClick)
    document.removeEventListener('keydown', onInspectorKeyDown)
  }

  const onInspectorDocClick = (e) => {
    if (inspectorMenu.contains(e.target) || inspectorBtn.contains(e.target)) return
    hideInspectorMenu()
  }
  const onInspectorKeyDown = (e) => {
    if (e.key === 'Escape') hideInspectorMenu()
  }

  inspectorBtn.addEventListener('click', (e) => {
    e.stopPropagation()
    if (!inspectorMenu.hidden) {
      hideInspectorMenu()
      return
    }
    renderInspectorMenu()
    inspectorMenu.hidden = false
    const rect = inspectorMenu.getBoundingClientRect()
    const btnRect = inspectorBtn.getBoundingClientRect()
    inspectorMenu.style.top = `${btnRect.bottom + 6}px`
    inspectorMenu.style.right = `${Math.max(8, window.innerWidth - btnRect.right)}px`
    document.addEventListener('click', onInspectorDocClick)
    document.addEventListener('keydown', onInspectorKeyDown)
  })

  // Hide the inspector button entirely when WebKit devtools aren't compiled in.
  api.inspectorAvailable()
    .then(ok => { if (!ok) inspectorBtn.hidden = true })
    .catch(() => {})

  const winMaxBtn = document.getElementById('win-max')
  async function syncMaximiseState() {
    try {
      const maximised = await api.windowIsMaximised()
      winMaxBtn.classList.toggle('maximised', !!maximised)
    } catch { /* noop */ }
  }
  document.getElementById('win-min').addEventListener('click', () => api.windowMinimise().catch(() => {}))
  winMaxBtn.addEventListener('click', async () => {
    try {
      await api.windowToggleMaximise()
    } catch { /* noop */ }
    syncMaximiseState()
  })
  document.getElementById('win-close').addEventListener('click', () => api.windowQuit().catch(() => {}))
  winMaxBtn.addEventListener('dblclick', syncMaximiseState)
  syncMaximiseState()

  loadServiceStatus()
  scheduleStatusPoll()

  let currentThemePref = 'system'

  async function applyTheme() {
    try {
      currentThemePref = await api.getTheme()
    } catch { /* keep default */ }
    return applyThemePref(currentThemePref)
  }

  function applyThemePref(pref) {
    currentThemePref = ['system', 'dark', 'light'].includes(pref) ? pref : 'system'
    if (currentThemePref === 'system') {
      return api.getSystemTheme()
        .then(sys => {
          document.documentElement.dataset.theme = sys === 'light' ? 'light' : 'dark'
        })
        .catch(() => {
          document.documentElement.dataset.theme = 'dark'
        })
    }
    document.documentElement.dataset.theme = currentThemePref === 'light' ? 'light' : 'dark'
    return Promise.resolve()
  }

  // Applies a preference picked from the Settings modal UI directly.
  function applyThemeUI(pref) {
    return applyThemePref(pref)
  }

  function openSettings() {
    settingsModal.setTheme(currentThemePref)
    settingsModal.open(dashboardStore.getTabs())
  }

  document.addEventListener('keydown', (e) => {
    const ctrl = e.ctrlKey || e.metaKey
    if (!ctrl) return
    const tabs = dashboardStore.getTabs()
    if (tabs.length === 0) return

    const activeIdx = tabs.findIndex(t => String(t.id) === String(activePanelTabId))

    if (e.key === 'Tab') {
      e.preventDefault()
      const dir = e.shiftKey ? -1 : 1
      const base = activeIdx === -1 ? (e.shiftKey ? tabs.length : -1) : activeIdx
      const nextIdx = (base + dir + tabs.length) % tabs.length
      switchTab(tabs[nextIdx].id)
    } else if (e.key.toLowerCase() === 't' && !e.shiftKey) {
      e.preventDefault()
      openSettings()
    } else if (e.key === '=' || e.key === '+' || e.key === '-' || e.key === '_' || e.key === '0') {
      const activeTab = tabs[activeIdx]
      if (!activeTab) return
      e.preventDefault()
      if (e.key === '=' || e.key === '+') setTabZoom(activeTab, zoomOf(activeTab) + ZOOM_STEP)
      else if (e.key === '-' || e.key === '_') setTabZoom(activeTab, zoomOf(activeTab) - ZOOM_STEP)
      else setTabZoom(activeTab, 1)
    }
  })

  await loadTabs()

  // Pause/resume the visible panel's auto-refresh when the window is hidden,
  // saving network traffic and CPU while the user is away.
  document.addEventListener('visibilitychange', () => {
    const view = currentViewId != null ? viewCache.get(String(currentViewId)) : null
    if (!view || !view.panel) return
    if (document.hidden) {
      view.panel.stopAutoRefresh()
    } else {
      view.panel.refresh()
      view.panel.startAutoRefresh()
    }
  })
}