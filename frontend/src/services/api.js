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

  async windowUnmaximise() {
    return runtime.WindowUnmaximise()
  },

  async windowIsMaximised() {
    return runtime.WindowIsMaximised()
  },

  async windowQuit() {
    return runtime.Quit()
  },

  async getDashboard() {
    return wails.GetDashboardNoContext()
  },

  async getSystemTheme() {
    return wails.GetSystemThemeNoContext()
  },

  async getTheme() {
    return wails.GetThemeNoContext()
  },

  async setTheme(theme) {
    return wails.SetThemeNoContext(theme)
  },

  async getServicesStatus() {
    return wails.GetServicesStatusNoContext()
  },

  async getNeuroNetData() {
    return wails.GetNeuroNetDataNoContext()
  },

  async getMinecraftData() {
    return wails.GetMinecraftDataNoContext()
  },

  async getSlotBuilderData() {
    return wails.GetSlotBuilderDataNoContext()
  },

  async proxyRequest(req) {
    return wails.ProxyRequest(req)
  },

  async getTabs() {
    return wails.ListTabsNoContext()
  },

  async saveTabConfig(config) {
    // config: { id, label, icon, url, enabled }
    return wails.AddTabNoContext(config)
  },

  async removeTab(tabId) {
    return wails.RemoveTabNoContext(String(tabId))
  },

  async updateTab(tabId, config) {
    // config: { id, label, icon, url, enabled }
    return wails.UpdateTabNoContext(String(tabId), config)
  },

  async updateTabSettings(tabId, settings) {
    // settings: { zoom, ... } — per-tab display options
    return wails.UpdateTabSettingsNoContext(String(tabId), settings || {})
  },

  async getNotes(tabId) {
    return wails.GetNotesNoContext(String(tabId))
  },

  async saveNotes(tabId, notes) {
    return wails.SaveNotesNoContext(String(tabId), notes || '')
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

  async shellSetChromeHeight(height) {
    return wails.ShellSetChromeHeightNoContext(Number(height))
  },

  async openSettings() {
    return wails.OpenSettingsNoContext()
  },

  async closeSettings() {
    return wails.CloseSettingsNoContext()
  },

  // Dedicated notes editor window (floating, non-modal — main window stays usable).
  async openNotes(tabId) {
    return wails.OpenNotesNoContext(Number(tabId))
  },

  async closeNotes() {
    return wails.CloseNotesNoContext()
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

  async listCookies(domain = '') {
    return wails.ListCookiesNoContext(domain)
  },

  async setCookie(cookie) {
    return wails.SetCookieNoContext(cookie)
  },

  async deleteCookie(domain, path, name) {
    return wails.DeleteCookieNoContext(domain, path, name)
  },

  async clearCookies(domain = '') {
    return wails.ClearCookiesNoContext(domain)
  },

  async neuronetInference(modelId, input) {
    return wails.NeuroNetInference(modelId, input)
  },

  async minecraftConsoleCommand(serverId, command) {
    return wails.MinecraftConsoleCommand(serverId, command)
  },
}