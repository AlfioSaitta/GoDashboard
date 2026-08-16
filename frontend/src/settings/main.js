// Settings window entry point (separate vite entry, served at
// "wails://wails/settings.html"). This page deliberately does NOT import the
// Wails runtime/bindings: Wails delivers Go->JS responses only to the main
// webview, so a second webview would hang. Instead every Go call goes through
// the WebKit message handler "dashboardSettings" (registered in tabs_shell.go)
// and responses arrive via window.__dashReply (run from settings_bridge.go).
import '../styles/main.css'
import { SettingsModal } from '../components/SettingsModal/SettingsModal.js'

const pending = new Map()
let seq = 0

function call(method, ...args) {
  return new Promise((resolve, reject) => {
    const id = ++seq
    pending.set(id, { resolve, reject })
    window.webkit.messageHandlers.dashboardSettings.postMessage(
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

const settingsApi = {
  getTheme: () => call('getTheme'),
  getSystemTheme: () => call('getSystemTheme'),
  setTheme: (theme) => call('setTheme', theme),
  getTabs: () => call('getTabs'),
  saveTabConfig: (config) => call('saveTabConfig', config),
  removeTab: (id) => call('removeTab', String(id)),
  updateTab: (id, config) => call('updateTab', String(id), config),
  updateTabSettings: (id, settings) => call('updateTabSettings', String(id), settings),
  reorderTabs: (ids) => call('reorderTabs', ids),
  tabsChanged: () => call('tabsChanged'),
  closeSettings: () => call('closeSettings'),
  shellZoom: (id, level) => call('shellZoom', Number(id), Number(level)),
  resize: (w, h) => call('resize', Math.round(w), Math.round(h)),
}

// ── Theme (mirrors the chrome logic) ──────────────────────
let themePref = 'system'

function applyThemePref(pref) {
  themePref = ['system', 'dark', 'light'].includes(pref) ? pref : 'system'
  if (themePref === 'system') {
    return settingsApi
      .getSystemTheme()
      .then((sys) => {
        document.documentElement.dataset.theme = sys === 'light' ? 'light' : 'dark'
      })
      .catch(() => {
        document.documentElement.dataset.theme = 'dark'
      })
  }
  document.documentElement.dataset.theme = themePref === 'light' ? 'light' : 'dark'
  return Promise.resolve()
}

async function mount() {
  try {
    themePref = await settingsApi.getTheme()
  } catch { /* default */ }
  await applyThemePref(themePref)

  document.getElementById('loading').remove()

  const app = document.getElementById('app')
  app.innerHTML =
    '<div id="settings-modal" class="modal-overlay" role="dialog" aria-modal="true" aria-labelledby="settings-title"></div>'
  document.body.classList.add('settings-mode')
  if (new URLSearchParams(window.location.search).get('t') === '1') {
    document.body.classList.add('dash-transparent')
  }

  const refreshList = async () => {
    try {
      settingsModal.open(await settingsApi.getTabs())
    } catch (error) {
      console.error('Failed to load tabs:', error)
    }
    scheduleFit()
  }

  // Sizes the native (transparent) window to the modal card. The measurement
  // must happen once layout is done, so it retries until the modal exists (the
  // modal may take a moment to render).
  let fitTimer = 0
  let lastFit = ''
  function scheduleFit(attempt = 0) {
    clearTimeout(fitTimer)
    fitTimer = window.setTimeout(() => {
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          const modal = document.querySelector('.modal')
          if (!modal) {
            if (attempt < 12) scheduleFit(attempt + 1)
            return
          }
          const r = modal.getBoundingClientRect()
          const w = Math.min(Math.max(Math.round(r.width) + 24, 400), 1200)
          const h = Math.min(Math.round(r.height) + 24, 900)
          const key = `${w}x${h}`
          if (key === lastFit) return
          lastFit = key
          settingsApi.resize(w, h).catch(() => {})
        })
      })
    }, 120)
  }

  const settingsModal = new SettingsModal(document.getElementById('settings-modal'), {
    onSave: async (config) => {
      await settingsApi.saveTabConfig(config)
      settingsApi.tabsChanged().catch(() => {})
      await refreshList()
    },
    onUpdateTab: async (tabId, config) => {
      await settingsApi.updateTab(tabId, config)
      settingsApi.tabsChanged().catch(() => {})
      await refreshList()
    },
    onRemoveTab: async (tabId) => {
      await settingsApi.removeTab(tabId)
      settingsApi.tabsChanged().catch(() => {})
      await refreshList()
    },
    onReorder: async (ids) => {
      try {
        await settingsApi.reorderTabs(ids)
        settingsApi.tabsChanged().catch(() => {})
      } catch (error) {
        console.error('Failed to reorder tabs:', error)
      }
    },
    onThemeChange: async (theme) => {
      try {
        await settingsApi.setTheme(theme)
        await applyThemePref(theme)
      } catch (error) {
        console.error('Failed to set theme:', error)
      }
    },
    onUpdateSettings: async (tabId, settings) => {
      if (settings && typeof settings.zoom === 'number') {
        settingsApi.shellZoom(tabId, settings.zoom).catch(() => {})
      }
      await settingsApi.updateTabSettings(tabId, settings)
    },
    onClose: () => {
      settingsApi.closeSettings().catch(() => {})
    },
  })

  settingsModal.setTheme(themePref)
  await refreshList()
  scheduleFit()
}

mount()