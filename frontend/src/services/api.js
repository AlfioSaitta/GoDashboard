// Wails v2 generated bindings
import * as wails from '../../wailsjs/go/main/App.js'
import * as runtime from '../../wailsjs/runtime/runtime.js'

export const api = {
  async windowMinimise() {
    return runtime.WindowMinimise()
  },

  async windowToggleMaximise() {
    return runtime.WindowToggleMaximise()
  },

  async windowIsMaximised() {
    return runtime.WindowIsMaximised()
  },

  async windowQuit() {
    return runtime.Quit()
  },

  async getSystemTheme() {
    return wails.GetSystemThemeNoContext()
  },

  async getTheme() {
    return wails.GetThemeNoContext()
  },

  async getServicesStatus() {
    return wails.GetServicesStatusNoContext()
  },

  async getTabs() {
    return wails.ListTabsNoContext()
  },

  async saveTabConfig(config) {
    // config: { id, label, icon, url, enabled }
    return wails.AddTabNoContext(config)
  },

  async updateTab(tabId, config) {
    // config: { id, label, icon, url, enabled }
    return wails.UpdateTabNoContext(String(tabId), config)
  },

  async updateTabSettings(tabId, settings) {
    // settings: { zoom, ... } — per-tab display options
    return wails.UpdateTabSettingsNoContext(String(tabId), settings || {})
  },

  // ── Native tab shell ──────────────────────────────────
  async shellShowTab(tabId) {
    return wails.ShellShowTabNoContext(Number(tabId))
  },

  async shellDestroyTab(tabId) {
    return wails.ShellDestroyTabNoContext(Number(tabId))
  },

  async shellReorder(ids) {
    return wails.ShellReorderNoContext(ids.map(id => Number(id)))
  },

  async shellZoom(tabId, level) {
    return wails.ShellZoomNoContext(Number(tabId), Number(level))
  },

  async shellNav(tabId, action) {
    // action: 'back' | 'forward' | 'reload' | 'stop' (id<=0 targets the chrome strip)
    const actions = {
      back: wails.ShellBackNoContext,
      forward: wails.ShellForwardNoContext,
      reload: wails.ShellReloadNoContext,
      stop: wails.ShellStopNoContext,
    }
    const fn = actions[action]
    if (!fn) return
    return fn(Number(tabId))
  },

  async shellSetChromeHeight(height) {
    return wails.ShellSetChromeHeightNoContext(Number(height))
  },

  async openSettings() {
    return wails.OpenSettingsNoContext()
  },

  // Dedicated notes editor window (floating, non-modal — main window stays usable).
  async openNotes(tabId) {
    return wails.OpenNotesNoContext(Number(tabId))
  },

  // Per-tab SSH terminal (native VTE in the tab box).
  async terminalToggle(tabId) {
    return wails.TerminalToggleNoContext(Number(tabId))
  },

  async terminalOpen(tabId) {
    return wails.TerminalOpenNoContext(Number(tabId))
  },

  async terminalClose(tabId) {
    return wails.TerminalCloseNoContext(Number(tabId))
  },

  async terminalRestart(tabId) {
    return wails.TerminalRestartNoContext(Number(tabId))
  },

  // Split orientation for the per-tab terminal: "h" (horizontal) or "v" (vertical).
  async terminalSplit(tabId, orient) {
    return wails.TerminalSplitNoContext(Number(tabId), orient)
  },

  async tabsChanged() {
    return wails.TabsChangedNoContext()
  },

  async inspectorAvailable() {
    return wails.InspectorAvailableNoContext()
  },

  async inspectorOpen(mode, tabId = 0) {
    // mode: 'bottom' | 'right' | 'left' | 'float'; tabId: tab whose page to inspect
    return wails.InspectorOpenNoContext(mode, Number(tabId))
  },

  async inspectorClose(tabId = 0) {
    return wails.InspectorCloseNoContext(Number(tabId))
  },

  async reorderTabs(ids) {
    return wails.ReorderTabsNoContext(ids.map(id => Number(id)))
  },

  async openExternal(url) {
    return wails.OpenExternalNoContext(url)
  },
}