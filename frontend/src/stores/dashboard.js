const DEFAULT_TAB_KEY = 'dashboard_default_tab'

class DashboardStore {
  constructor() {
    this.tabs = []
    this.lastStatuses = []
    this.listeners = new Set()
  }

  getTabs() {
    return this.tabs
  }

  getDefaultTab() {
    return localStorage.getItem(DEFAULT_TAB_KEY) || 'neuronet'
  }

  setDefaultTab(tabId) {
    localStorage.setItem(DEFAULT_TAB_KEY, String(tabId))
  }

  setTabs(tabs) {
    this.tabs = Array.isArray(tabs) ? tabs : []
    this.notify()
  }

  subscribe(listener) {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  notify() {
    this.listeners.forEach(l => l(this.tabs))
  }
}

export const dashboardStore = new DashboardStore()