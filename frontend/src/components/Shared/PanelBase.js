import { createElement, icon, escapeHtml } from './utils.js'

// Shared panel lifecycle. Subclasses provide: contentHtml(). The base
// handles loading/error/inline-error states, the refresh button, the
// auto-refresh interval (idempotent) and the per-property "updated" stamp.
//
// No-flicker refresh: on the FIRST load a spinner is shown; on subsequent
// refreshes the existing content stays in place while new data is fetched, so
// the panel does not flash "Caricamento..." every auto-refresh cycle.
export class PanelBase {
  constructor({ iconName, title, bindRefresh, panelClass = 'base-panel', refreshMs = 30000 }) {
    this.iconName = iconName
    this.title = title
    this.bindRefresh = bindRefresh // () => Promise<data>
    this.panelClass = panelClass
    this.refreshMs = refreshMs
    this.element = null
    this.data = null
    this.refreshInterval = null
    this.refreshing = false
    this.rendered = false
  }

  render() {
    this.element = createElement(`
      <div class="panel ${this.panelClass}">
        <div class="panel-header">
          <div class="panel-title">
            ${icon(this.iconName, 24)}
            <h2>${this.title}</h2>
          </div>
          <div class="panel-actions">
            <span class="panel-updated" id="panel-updated" hidden></span>
            <button class="btn btn-icon btn-refresh" id="refresh-btn" aria-label="Aggiorna">${icon('refresh', 16)}</button>
          </div>
        </div>
        <div class="panel-content" id="panel-content">
          <div class="loading">Caricamento...</div>
        </div>
      </div>
    `)
    return this.element
  }

  mount(container) {
    this.container = container
    this.element.querySelector('#refresh-btn').addEventListener('click', () => this.refresh())
    this.startAutoRefresh()
  }

  unmount() {
    this.stopAutoRefresh()
  }

  async refresh() {
    if (this.refreshing) return // never overlap refreshes
    this.refreshing = true
    try {
      const content = this.element.querySelector('#panel-content')
      // Keep stale content visible while reloading; only show the spinner on
      // the very first load.
      if (!this.rendered) {
        content.innerHTML = '<div class="loading">Caricamento...</div>'
      }
      this.data = await this.bindRefresh()
      content.innerHTML = this.contentHtml()
      this.rendered = true
      this.#stamp()
    } catch (error) {
      if (!this.rendered) {
        this.showError(error.message, content)
      } else {
        this.showInlineError(error.message, content)
      }
    } finally {
      this.refreshing = false
    }
  }

  contentHtml() {
    return '<div class="empty-state">Nessun dato disponibile</div>'
  }

  showError(message, content) {
    content.innerHTML = `
      <div class="error-state">
        ${icon('alert', 48)}
        <h3>Errore nel caricamento</h3>
        <p>${escapeHtml(message)}</p>
        <button class="btn btn-primary" id="retry-btn">Riprova</button>
      </div>
    `
    content.querySelector('#retry-btn').addEventListener('click', () => this.refresh())
  }

  // Shows a subtle inline banner while keeping the previously loaded content.
  showInlineError(message, content) {
    let banner = content.querySelector('.panel-inline-error')
    if (!banner) {
      banner = document.createElement('div')
      banner.className = 'panel-inline-error'
      content.prepend(banner)
    }
    banner.innerHTML = `${icon('alert', 14)} <span class="panel-inline-msg">${escapeHtml(message)}</span>`
  }

  // "Last updated" stamp in the panel header, refreshed on each render.
  #stamp() {
    const el = this.element.querySelector('#panel-updated')
    if (!el) return
    el.textContent = formatUpdated(new Date())
    el.hidden = false
  }

  startAutoRefresh() {
    if (this.refreshInterval) return // idempotent: no duplicate timers
    this.refreshInterval = setInterval(() => this.refresh(), this.refreshMs ?? 30000)
  }

  stopAutoRefresh() {
    if (this.refreshInterval) {
      clearInterval(this.refreshInterval)
      this.refreshInterval = null
    }
  }
}

export function formatUpdated(date) {
  return 'aggiornato ' + date.toLocaleTimeString('it-IT', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}