import { TabBar } from '@/components/TabBar/TabBar.js'
import { SettingsModal } from '@/components/SettingsModal/SettingsModal.js'
import { dashboardStore } from '@/stores/dashboard.js'
import { api } from '@/services/api.js'
import { icon } from '@/components/Shared/utils.js'
import { urlForTab } from '@/components/Shared/services.js'
import * as runtime from '../wailsjs/runtime/runtime.js'

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

// ── Theme ────────────────────────────────────────────────
let currentThemePref = 'system'

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

async function applyTheme() {
  try {
    currentThemePref = await api.getTheme()
  } catch { /* keep default */ }
  return applyThemePref(currentThemePref)
}

export async function createApp() {
  const settingsView = window.location.hash === '#settings'
  if (settingsView) {
    await mountSettingsView()
    return
  }
  await mountChrome()
}

// ── Impostazioni: rendered as the WHOLE content of the native settings
// window (the app bundle is loaded with "#settings").
async function mountSettingsView() {
  try {
    await applyTheme()
  } catch { /* keep default */ }

  const app = document.getElementById('app')
  app.innerHTML = '<div id="settings-modal" class="modal-overlay" role="dialog" aria-modal="true" aria-labelledby="settings-title"></div>'
  document.body.classList.add('settings-mode')

  const refreshList = async () => {
    try {
      settingsModal.open(await api.getTabs())
    } catch { /* keep list */ }
  }

  const settingsModal = new SettingsModal(document.getElementById('settings-modal'), {
    onSave: async (config) => {
      await api.saveTabConfig(config)
      api.tabsChanged().catch(() => {})
      await refreshList()
    },
    onUpdateTab: async (tabId, config) => {
      await api.updateTab(tabId, config)
      api.tabsChanged().catch(() => {})
      await refreshList()
    },
    onRemoveTab: async (tabId) => {
      await api.removeTab(tabId)
      api.tabsChanged().catch(() => {})
      await refreshList()
    },
    onReorder: async (ids) => {
      try {
        await api.reorderTabs(ids)
        api.tabsChanged().catch(() => {})
      } catch (error) {
        console.error('Failed to reorder tabs:', error)
      }
    },
    onThemeChange: async (theme) => {
      try {
        await api.setTheme(theme)
        await applyThemePref(theme)
      } catch (error) {
        console.error('Failed to save theme:', error)
      }
    },
    onUpdateSettings: async (tabId, settings) => {
      try {
        if (settings && typeof settings.zoom === 'number') {
          api.shellZoom(tabId, settings.zoom).catch(() => {})
        }
        await api.updateTabSettings(tabId, settings)
      } catch (error) {
        console.error('Failed to save tab settings:', error)
      }
    },
    onNotesChange: async (tabId, notes) => {
      try {
        await api.saveNotes(tabId, notes)
      } catch (error) {
        console.error('Failed to save notes:', error)
      }
    },
    onClose: async () => {
      try { await api.closeSettings() } catch { /* noop */ }
    },
  })

  const tabs = await api.getTabs()
  settingsModal.setTheme(currentThemePref)
  settingsModal.open(tabs || [])
}

// ── Chrome strip: header + tab bar only. Each tab is a native webview
// managed by the Go shell (see tabs_shell.go / ShellShowTab).
async function mountChrome() {
  try {
    await applyTheme()
  } catch { /* keep default */ }

  const app = document.getElementById('app')
  app.innerHTML = `
    <div class="dashboard strip">
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
    </div>
  `

  let currentTabId = null
  let stripHeight = 104
  let stripExpanded = false
  const knownTabIds = new Set()

  // When a DOM popup (tab context menu, inspector dropdown) is open the chrome
  // webview is grown so the popup is not clipped by the thin strip. The height
  // is kept moderate so the tab pages (the GtkStack below the strip) stay
  // visible instead of being collapsed to nothing. NOTE: the notes editor is a
  // dedicated native window (see openNotes), never a DOM popup.
  const EXPANDED_STRIP = 480

  // ── Chrome strip height (the Go shell sizes the chrome webview exactly to
  // header + tab bar, leaving the rest of the window to the tab stack).
  let heightSyncTimer = null
  function measureStripHeight() {
    const bar = document.getElementById('tab-bar')
    if (!bar) return stripHeight
    return Math.max(60, Math.ceil(bar.getBoundingClientRect().bottom) + 2)
  }

  async function applyChromeHeight(h) {
    if (stripExpanded) return
    stripHeight = h || measureStripHeight()
    api.shellSetChromeHeight(stripHeight).catch(() => {})
  }

  function syncChromeHeight() {
    if (stripExpanded) return
    clearTimeout(heightSyncTimer)
    heightSyncTimer = setTimeout(() => applyChromeHeight(), 30)
  }

  // Temporarily grow the chrome webview so DOM popups (context menu, inspector
  // dropdown) are not clipped by the thin strip; shrink back once they close.
  function expandStrip() {
    if (stripExpanded) return
    stripExpanded = true
    api.shellSetChromeHeight(EXPANDED_STRIP).catch(() => {})
  }

  function collapseStrip() {
    if (!stripExpanded) return
    stripExpanded = false
    api.shellSetChromeHeight(stripHeight).catch(() => {})
  }

  // ── Per-tab notes ───────────────────────────────────────
  // The notes editor is a DEDICATED floating window (like the Impostazioni
  // window: its own webview + "dashboardNotes" bridge), so the chrome strip is
  // never resized and the tab pages below never shift while it is open.
  function openNotes(tab) {
    api.openNotes(tab.id).catch(err => console.error('Failed to open notes:', err))
  }

  function setTabZoom(tab, z) {
    const current = (tab.settings && typeof tab.settings === 'object') ? tab.settings : {}
    const merged = { ...current, zoom: clampZoom(z) }
    tab.settings = merged
    api.shellZoom(tab.id, clampZoom(z)).catch(() => {})
    api.updateTabSettings(tab.id, merged).catch(err => console.error('Failed to save tab settings:', err))
  }

  const tabBar = new TabBar(document.getElementById('tab-bar'), {
    onTabChange: switchTab,
    onAddTab: () => api.openSettings().catch(() => {}),
    onReorder: async (ids) => {
      try {
        await api.reorderTabs(ids)
        api.shellReorder(ids).catch(() => {})
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
      // Context menu on the (expanded) strip: the page is a native webview, so
      // the transitory URL shown is the resolved one.
      const url = resolveUrlForTooltip(tab)
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
    onNotes: (tab) => openNotes(tab),
    onTerminal: (tab) => {
      api.terminalToggle(tab.id).catch(err => console.error('Failed to toggle terminal:', err))
    },
    onPopupChange: (open) => { if (open) expandStrip(); else collapseStrip() },
  })

  // Per-tab persistent notes editor is now a dedicated native window (see the
  // notes bridge + tabs_shell.go notes window); the chrome only refreshes the
  // note indicator via the "tabs:changed" event after a save.

  // A tab was activated outside the chrome (e.g. from the tray context menu):
  // highlight it in the tab bar and re-assert the shell tab.
  runtime.EventsOn('shell:tab-activated', (data) => {
    if (!data || data.tabId == null) return
    switchTab(data.tabId)
  })

  // Native webviews report their page title; show it transiently on the tab.
  runtime.EventsOn('shell:title', (data) => {
    if (!data || data.tabId == null) return
    tabBar.setPageTitle(String(data.tabId), data.title)
  })

  // The Impostazioni window notifies us whenever the tab list changed.
  runtime.EventsOn('tabs:changed', () => {
    loadTabs({ preserveActive: true }).catch(() => {})
  })

  // The Impostazioni window applies a new theme preference.
  runtime.EventsOn('shell:theme', (theme) => {
    applyThemePref(theme).catch(() => {})
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

  function switchTab(tabId) {
    const tab = dashboardStore.getTabs().find(t => String(t.id) === String(tabId))
    if (!tab) {
      currentTabId = null
      tabBar.setActive(null)
      return
    }
    currentTabId = tab.id
    tabBar.setActive(tab.id)
    api.shellShowTab(tab.id).catch(err => console.error('Failed to show tab:', err))
  }

  async function loadTabs({ preserveActive = false } = {}) {
    try {
      const tabs = await api.getTabs()
      const list = tabs || []

      dashboardStore.setTabs(list)
      tabBar.setStatuses(dashboardStore.lastStatuses || [])
      tabBar.setTabs(list)
      tabBar.setDefaultTabId(dashboardStore.getDefaultTab())

      // Destroy the native webviews of removed tabs (their session is gone).
      const alive = new Set(list.map(t => String(t.id)))
      knownTabIds.forEach(id => {
        if (!alive.has(id)) {
          api.shellDestroyTab(id).catch(() => {})
          knownTabIds.delete(id)
        }
      })
      list.forEach(t => knownTabIds.add(String(t.id)))

      if (list.length > 0) {
        let target
        if (preserveActive && currentTabId != null && alive.has(String(currentTabId))) {
          target = currentTabId
        } else {
          const defaultTabId = dashboardStore.getDefaultTab()
          target = list.find(t => String(t.id) === String(defaultTabId)) ? defaultTabId : list[0].id
        }
        switchTab(target)
      } else {
        currentTabId = null
        tabBar.setActive(null)
      }
    } catch (error) {
      console.error('Failed to load tabs:', error)
      currentTabId = null
      tabBar.setActive(null)
    }
  }

  function resolveUrlForTooltip(tab) {
    return urlForTab(tab)
  }

  // ── Factory glue: computed label of the active tab for the inspector. ──
  function activeTab() {
    return dashboardStore.getTabs().find(t => String(t.id) === String(currentTabId)) || null
  }

  document.getElementById('settings-btn').addEventListener('click', () => {
    api.openSettings().catch(() => {})
  })

  // ── Inspector dropdown (page of the ACTIVE tab, mode selectable) ──────
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
        const tab = activeTab()
        const tabId = tab ? tab.id : 0
        try {
          if (mode === 'close') await api.inspectorClose(tabId)
          else await api.inspectorOpen(mode, tabId)
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
    collapseStrip()
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
    expandStrip()
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

  document.addEventListener('keydown', (e) => {
    const ctrl = e.ctrlKey || e.metaKey
    if (!ctrl) return
    const tabs = dashboardStore.getTabs()
    if (tabs.length === 0) return

    const activeIdx = tabs.findIndex(t => String(t.id) === String(currentTabId))

    if (e.key === 'Tab') {
      e.preventDefault()
      const dir = e.shiftKey ? -1 : 1
      const base = activeIdx === -1 ? (e.shiftKey ? tabs.length : -1) : activeIdx
      const nextIdx = (base + dir + tabs.length) % tabs.length
      switchTab(tabs[nextIdx].id)
    } else if (e.key.toLowerCase() === 't' && !e.shiftKey) {
      e.preventDefault()
      api.openSettings().catch(() => {})
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
  syncChromeHeight()
  window.addEventListener('resize', syncChromeHeight)
}