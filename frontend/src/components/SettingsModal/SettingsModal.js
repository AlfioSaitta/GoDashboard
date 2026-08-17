import { icon, escapeHtml } from '../Shared/utils.js'
import { tabZoom } from '../Shared/services.js'

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

const SECTIONS = [
  ['services', 'Servizi'],
  ['prefs', 'Preferenze'],
]

const AUTH_TYPES = [
  ['none', 'Nessuna'],
  ['basic', 'Basic (user/password)'],
  ['bearer', 'Bearer token'],
]

const TERM_AUTHS = [
  ['agent', 'SSH agent'],
  ['password', 'Password (env)'],
  ['key', 'Chiave privata (file)'],
]

const NEW_ID = 'new'

export class SettingsModal {
  constructor(overlayEl, {
    onSave, onUpdateTab, onRemoveTab, onClose, onReorder,
    onThemeChange, onUpdateSettings, onNotesChange,
    onSaveService, onSaveGlobal, onFit,
  }) {
    this.overlay = overlayEl
    this.onSave = onSave
    this.onUpdateTab = onUpdateTab
    this.onRemoveTab = onRemoveTab
    this.onClose = onClose
    this.onReorder = onReorder
    this.onThemeChange = onThemeChange
    this.onUpdateSettings = onUpdateSettings
    this.onNotesChange = onNotesChange
    this.onSaveService = onSaveService
    this.onSaveGlobal = onSaveGlobal
    this.onFit = onFit
    this.tabs = []
    this.appConfig = null
    this.section = 'services'
    this.editingId = null
    this.draggingId = null
    this.removeCandidateId = null
    this.render()
  }

  render() {
    this.overlay.innerHTML = `
      <div class="modal" role="document">
        <header class="modal-header">
          <h2 id="settings-title">${icon('settings', 20)} Impostazioni</h2>
          <button class="btn btn-icon btn-close" id="close-modal" aria-label="Chiudi">${icon('close', 18)}</button>
        </header>
        <nav class="settings-nav" aria-label="Sezioni impostazioni">
          ${SECTIONS.map(([key, label]) => `
            <button class="settings-nav-btn${this.section === key ? ' active' : ''}" data-section="${key}" type="button">${icon(this.sectionIcon(key), 15)} ${label}</button>
          `).join('')}
        </nav>
        <div class="modal-body">
          <section class="settings-section" data-section-panel="services"></section>
          <section class="settings-section" data-section-panel="prefs" hidden></section>
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

    this.overlay.querySelectorAll('.settings-nav-btn').forEach(btn => {
      btn.addEventListener('click', () => this.switchSection(btn.dataset.section))
    })

    this.renderServicesSection()
    this.renderPrefsSection()
  }

  sectionIcon(key) {
    return key === 'services' ? 'server' : 'sliders'
  }

  switchSection(key) {
    this.section = SECTIONS.some(([k]) => k === key) ? key : 'services'
    this.overlay.querySelectorAll('.settings-nav-btn').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.section === this.section)
    })
    this.overlay.querySelectorAll('[data-section-panel]').forEach(panel => {
      panel.hidden = panel.dataset.sectionPanel !== this.section
    })
    this.hideError()
    if (this.onFit) this.onFit()
  }

  // ── Preferences section ──────────────────────────────

  renderPrefsSection() {
    const panel = this.overlay.querySelector('[data-section-panel="prefs"]')
    panel.innerHTML = `
      <div class="settings-card">
        <h3>${icon('sliders', 14)} Preferenze applicazione</h3>
        <div class="form-row">
          <div class="form-group">
            <label for="pref-theme">Tema</label>
            <select id="pref-theme" name="theme">
              <option value="system">Automatico (sistema)</option>
              <option value="dark">Scuro</option>
              <option value="light">Chiaro</option>
            </select>
          </div>
          <div class="form-group">
            <label for="pref-default-tab">Servizio predefinito</label>
            <select id="pref-default-tab" name="default_tab"></select>
          </div>
        </div>
        <div class="form-group">
          <label for="pref-gpu">Accelerazione WebView</label>
          <select id="pref-gpu" name="webview_gpu_policy">
            <option value="always">Sempre (hardware)</option>
            <option value="ondemand">Su richiesta</option>
            <option value="never">Mai (software)</option>
          </select>
        </div>
      </div>

      <div class="settings-card">
        <h3>${icon('server', 14)} Proxy</h3>
        <div class="form-group form-toggle">
          <label for="pref-proxy-enabled">
            <input type="checkbox" id="pref-proxy-enabled" name="proxy_enabled">
            <span>Proxy attivo</span>
          </label>
        </div>
        <div class="form-group">
          <label for="pref-proxy-hosts">Host permessi (uno per riga)</label>
          <textarea id="pref-proxy-hosts" name="allowed_hosts" rows="4" spellcheck="false" placeholder="es. localhost:8000&#10;51.75.77.248:9800"></textarea>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="pref-proxy-timeout">Timeout (secondi)</label>
            <input type="number" id="pref-proxy-timeout" name="timeout_seconds" min="1" max="600">
          </div>
          <div class="form-group">
            <label for="pref-proxy-maxbody">Body max (MB)</label>
            <input type="number" id="pref-proxy-maxbody" name="max_body_size_mb" min="1" max="1024">
          </div>
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-primary" id="pref-save-btn">${icon('check', 14)} Salva Preferenze</button>
        </div>
      </div>
    `

    panel.querySelectorAll('#pref-theme, #pref-default-tab, #pref-gpu').forEach(el => {
      el.addEventListener('change', () => this.hideError())
    })
    panel.querySelector('#pref-save-btn').addEventListener('click', () => this.saveGlobal())
  }

  setAppConfig(config) {
    this.appConfig = config || null
    this.renderServiceList()
    this.fillPrefs()
  }

  setTheme(theme) {
    const select = this.overlay.querySelector('#pref-theme')
    if (select && ['system', 'dark', 'light'].includes(theme)) select.value = theme
  }

  open(tabs, config) {
    this.tabs = tabs || []
    this.appConfig = config || null
    this.editingId = null
    this.removeCandidateId = null
    this.renderServicesSection()
    this.fillPrefs()
    this.overlay.classList.add('open')
    document.body.style.overflow = 'hidden'
    if (this.onFit) this.onFit()
  }

  close() {
    this.overlay.classList.remove('open')
    document.body.style.overflow = ''
    this.editingId = null
    this.removeCandidateId = null
    this.hideError()
    // In the native settings window "close" means closing the window itself.
    if (this.onClose) this.onClose()
  }

  // ── Preferences: fill + save ─────────────────────────

  fillPrefs() {
    const panel = this.overlay.querySelector('[data-section-panel="prefs"]')
    if (!panel || !this.appConfig) return
    const ui = this.appConfig.ui || {}
    const proxy = this.appConfig.proxy || {}
    const themeSelect = panel.querySelector('#pref-theme')
    if (themeSelect) themeSelect.value = ['system', 'dark', 'light'].includes(ui.theme) ? ui.theme : 'system'
    const gpuSelect = panel.querySelector('#pref-gpu')
    if (gpuSelect) gpuSelect.value = ['always', 'ondemand', 'never'].includes(ui.webview_gpu_policy) ? ui.webview_gpu_policy : 'always'
    const defSelect = panel.querySelector('#pref-default-tab')
    if (defSelect) {
      let current = ui.default_tab || ''
      if (!current && typeof localStorage !== 'undefined') current = localStorage.getItem('dashboard_default_tab') || ''
      defSelect.innerHTML = this.tabs.map(t =>
        `<option value="${escapeHtml(String(t.id))}"${String(t.id) === String(current) ? ' selected' : ''}>${escapeHtml(t.label)}</option>`
      ).join('')
    }
    const enabled = panel.querySelector('#pref-proxy-enabled')
    if (enabled) enabled.checked = !!proxy.enabled
    const hosts = panel.querySelector('#pref-proxy-hosts')
    if (hosts) hosts.value = (proxy.allowed_hosts || []).join('\n')
    const to = panel.querySelector('#pref-proxy-timeout')
    if (to) to.value = proxy.timeout_seconds ?? 60
    const mb = panel.querySelector('#pref-proxy-maxbody')
    if (mb) mb.value = proxy.max_body_size_mb ?? 50
  }

  async saveGlobal() {
    this.hideError()
    const panel = this.overlay.querySelector('[data-section-panel="prefs"]')
    try {
      const defaultTabEl = panel.querySelector('#pref-default-tab')
      const defaultTab = defaultTabEl ? defaultTabEl.value : (this.appConfig?.ui?.default_tab || '')
      if (defaultTab && typeof localStorage !== 'undefined') {
        localStorage.setItem('dashboard_default_tab', defaultTab)
      }
      const hosts = (panel.querySelector('#pref-proxy-hosts')?.value || '')
        .split('\n').map(s => s.trim()).filter(Boolean)
      const patch = {
        ui: {
          theme: panel.querySelector('#pref-theme')?.value || 'system',
          default_tab: defaultTab,
          webview_gpu_policy: panel.querySelector('#pref-gpu')?.value || 'always',
        },
        proxy: {
          enabled: panel.querySelector('#pref-proxy-enabled')?.checked || false,
          allowed_hosts: hosts,
          timeout_seconds: Number(panel.querySelector('#pref-proxy-timeout')?.value || 60),
          max_body_size_mb: Number(panel.querySelector('#pref-proxy-maxbody')?.value || 50),
        },
      }
      if (this.onSaveGlobal) await this.onSaveGlobal(patch)
      this.onThemeChange?.(patch.ui.theme)
      this.showToast('Preferenze salvate')
    } catch (error) {
      this.showError('Errore: ' + error.message)
    }
  }

  // ── Services section ─────────────────────────────────

  renderServicesSection() {
    const panel = this.overlay.querySelector('[data-section-panel="services"]')
    if (!panel) return
    panel.innerHTML = `
      <div class="settings-card">
        <div class="settings-card-head">
          <h3>${icon('server', 14)} Servizi</h3>
          <button type="button" class="btn btn-secondary btn-sm" id="add-service-btn">${icon('plus', 14)} Aggiungi</button>
        </div>
        <p class="settings-hint">Trascina le righe o usa le frecce per ordinare. Apri una riga per modificarne la voce e, se collegata, la configurazione del servizio.</p>
        <ul id="service-list" class="tab-list" aria-label="Elenco servizi"></ul>
      </div>
    `
    panel.querySelector('#add-service-btn').addEventListener('click', () => this.startNewTab())
    this.renderServiceList()
  }

  // The service a tab belongs to. Matches either by the tab URL being the
  // service KEY itself or by URL prefix against the service's base/backoffice/
  // frontend URLs (tabs store the resolved real URL, e.g.
  // "http://localhost:8000/admin", while config keys are "neuronet"...).
  serviceKeyForTab(tab) {
    if (!tab || !tab.url) return null
    const services = this.appConfig?.services || {}
    const url = String(tab.url)
    if (services[url]) return url
    const clean = url.replace(/\/+$/, '')
    for (const [key, svc] of Object.entries(services)) {
      const candidates = [svc.base_url, svc.backoffice_url, svc.frontend_url]
        .filter(Boolean)
        .map((u) => String(u).replace(/\/+$/, ''))
      for (const base of candidates) {
        if (clean === base || clean.startsWith(base)) return key
      }
    }
    return null
  }

  serviceForTab(tab) {
    const key = this.serviceKeyForTab(tab)
    return (key && this.appConfig?.services?.[key]) || null
  }

  // Renders ONE unified list: every tab is a row; a row whose URL is a service
  // key carries the full service configuration in its edit panel.
  renderServiceList() {
    const list = this.overlay.querySelector('#service-list')
    if (!list) return

    const rows = []
    if (this.editingId === NEW_ID) {
      rows.push(this.newRowHTML())
    }
    rows.push(...this.tabs.map((tab, idx) => this.tabRowHTML(tab, idx)))

    if (rows.length === 0) {
      list.innerHTML = '<li class="empty">Nessun servizio configurato</li>'
      return
    }
    list.innerHTML = rows.join('')

    this.attachServiceListEvents(list)
  }

  tabRowHTML(tab, idx) {
    const editing = this.editingId === String(tab.id)
    const svc = this.serviceForTab(tab)
    const subtitle = svc
      ? `servizio: ${escapeHtml(svc.base_url || svc.backoffice_url || svc.frontend_url || tab.url || '')}`
      : escapeHtml(tab.url || '—')
    return `
      <li class="tab-item" data-tab-id="${tab.id}" draggable="true">
        <div class="tab-info">
          <span class="tab-drag-handle">${icon('chevronDown', 12)}</span>
          <span class="tab-icon">${icon(tab.icon, 16)}</span>
          <div>
            <span class="tab-title">${escapeHtml(tab.label)}</span>
            <span class="tab-url">${subtitle}</span>
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
            <button class="btn btn-icon btn-icon-soft edit-tab" data-tab-id="${tab.id}" aria-label="Modifica ${tab.label}" title="Modifica">${icon('edit', 14)}</button>
            <button class="btn btn-icon btn-danger remove-tab" data-tab-id="${tab.id}" aria-label="Rimuovi ${tab.label}" title="Rimuovi">${icon('close', 14)}</button>
          `}
        </div>
      </li>
      ${editing ? this.editPanelHTML(tab) : ''}
    `
  }

  newRowHTML() {
    return `
      <li class="tab-item is-new" data-tab-id="${NEW_ID}">
        <div class="tab-info">
          <span class="tab-icon">${icon('plus', 16)}</span>
          <div><span class="tab-title">Nuovo servizio</span></div>
        </div>
        <div class="tab-item-actions">
          <button class="btn btn-icon btn-icon-soft cancel-new-btn" data-tab-id="${NEW_ID}" aria-label="Annulla" title="Annulla">${icon('close', 14)}</button>
        </div>
      </li>
      <li class="service-edit-panel is-new" data-tab-id="${NEW_ID}">
        <div class="form-row">
          <div class="form-group">
            <label for="tabf-label-${NEW_ID}">Etichetta</label>
            <input type="text" id="tabf-label-${NEW_ID}" value="" placeholder="Es. Mio Servizio" autocomplete="off">
          </div>
          <div class="form-group">
            <label for="tabf-icon-${NEW_ID}">Icona</label>
            <select id="tabf-icon-${NEW_ID}">
              ${ICONS.map(([value, label]) => `<option value="${value}">${label}</option>`).join('')}
            </select>
          </div>
        </div>
        <div class="form-group">
          <label for="tabf-url-${NEW_ID}">URL / chiave servizio</label>
          <input type="text" id="tabf-url-${NEW_ID}" value="" placeholder="https://example.com oppure neuronet / minecraft / slotbuilder" autocomplete="off">
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary cancel-new-btn" data-tab-id="${NEW_ID}">Annulla</button>
          <button type="button" class="btn btn-primary save-new-btn">${icon('check', 14)} Aggiungi</button>
        </div>
      </li>
    `
  }

  // Inline edit panel for an EXISTING row: tab fields + (if the tab URL is a
  // service key) the full service config + zoom + notes.
  editPanelHTML(tab) {
    const svc = this.serviceForTab(tab)
    const zoom = Math.round(tabZoom(tab) * 100)
    return `
      <li class="service-edit-panel" data-tab-id="${tab.id}">
        <div class="form-row">
          <div class="form-group">
            <label for="tabf-label-${tab.id}">Etichetta</label>
            <input type="text" id="tabf-label-${tab.id}" value="${escapeHtml(tab.label || '')}" autocomplete="off">
          </div>
          <div class="form-group">
            <label for="tabf-icon-${tab.id}">Icona</label>
            <select id="tabf-icon-${tab.id}">
              ${ICONS.map(([value, label]) => `<option value="${value}"${tab.icon === value ? ' selected' : ''}>${label}</option>`).join('')}
            </select>
          </div>
        </div>
        <div class="form-group">
          <label for="tabf-url-${tab.id}">${svc ? 'Chiave servizio' : 'URL'}</label>
          <input type="text" id="tabf-url-${tab.id}" value="${escapeHtml(tab.url || '')}" ${svc ? 'readonly class="is-readonly"' : ''} autocomplete="off" ${svc ? 'title="Collegato al servizio configurato sotto"' : ''}>
        </div>
        ${svc ? this.serviceConfigHTML(tab.url, svc) : ''}
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
        <div class="tab-settings-row tab-settings-notes">
          <label for="notes-textarea-${tab.id}">Note</label>
          <textarea id="notes-textarea-${tab.id}" class="tab-notes-input" data-tab-id="${tab.id}" rows="4" spellcheck="false" placeholder="Note persistenti per questo servizio…">${escapeHtml(tab.notes || '')}</textarea>
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary cancel-edit-btn" data-tab-id="${tab.id}">Annulla</button>
          <button type="button" class="btn btn-primary save-row-btn" data-tab-id="${tab.id}">${icon('check', 14)} Salva</button>
        </div>
      </li>
    `
  }

  serviceConfigHTML(key, svc) {
    const auth = svc.auth || {}
    const term = svc.terminal || {}
    return `
      <h4 class="service-subhead">Configurazione servizio</h4>
      <div class="form-group">
        <label for="sc-${key}-name">Nome servizio</label>
        <input type="text" id="sc-${key}-name" data-field="name" value="${escapeHtml(svc.name || '')}" autocomplete="off">
      </div>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-base">Base URL (API)</label>
          <input type="url" id="sc-${key}-base" data-field="base_url" value="${escapeHtml(svc.base_url || '')}" placeholder="https://api.example.com" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-prefix">Prefisso API</label>
          <input type="text" id="sc-${key}-prefix" data-field="api_prefix" value="${escapeHtml(svc.api_prefix || '/api')}" placeholder="/api" autocomplete="off">
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-backoffice">Backoffice URL</label>
          <input type="url" id="sc-${key}-backoffice" data-field="backoffice_url" value="${escapeHtml(svc.backoffice_url || '')}" placeholder="https://backoffice.example.com" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-frontend">Frontend URL</label>
          <input type="url" id="sc-${key}-frontend" data-field="frontend_url" value="${escapeHtml(svc.frontend_url || '')}" placeholder="https://example.com" autocomplete="off">
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-admin-path">Percorso admin</label>
          <input type="text" id="sc-${key}-admin-path" data-field="admin_path" value="${escapeHtml(svc.admin_path || '')}" placeholder="/admin" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-timeout">Timeout (secondi)</label>
          <input type="number" id="sc-${key}-timeout" data-field="timeout_seconds" min="1" max="600" value="${svc.timeout_seconds ?? 30}">
        </div>
      </div>
      <div class="form-toggle form-group">
        <label for="sc-${key}-proxy">
          <input type="checkbox" id="sc-${key}-proxy" data-field="proxy_enabled" ${svc.proxy_enabled ? 'checked' : ''}>
          <span>Proxy attivo per questo servizio</span>
        </label>
      </div>

      <h4 class="service-subhead">Autenticazione</h4>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-auth-type">Tipo</label>
          <select id="sc-${key}-auth-type" data-field="auth.type">
            ${AUTH_TYPES.map(([v, l]) => `<option value="${v}"${auth.type === v ? ' selected' : ''}>${l}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"></div>
      </div>
      <div class="form-row" data-auth-fields="basic">
        <div class="form-group">
          <label for="sc-${key}-auth-user">Env var utente</label>
          <input type="text" id="sc-${key}-auth-user" data-field="auth.username_env" value="${escapeHtml(auth.username_env || '')}" placeholder="MINECRAFT_USER" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-auth-pass">Env var password</label>
          <input type="text" id="sc-${key}-auth-pass" data-field="auth.password_env" value="${escapeHtml(auth.password_env || '')}" placeholder="MINECRAFT_PASS" autocomplete="off">
        </div>
      </div>
      <div class="form-group" data-auth-fields="bearer">
        <label for="sc-${key}-auth-token">Env var token</label>
        <input type="text" id="sc-${key}-auth-token" data-field="auth.token_env" value="${escapeHtml(auth.token_env || '')}" placeholder="SLOTBUILDER_TOKEN" autocomplete="off">
      </div>

      <h4 class="service-subhead">Terminale SSH</h4>
      <div class="form-toggle form-group">
        <label for="sc-${key}-term-enabled">
          <input type="checkbox" id="sc-${key}-term-enabled" data-field="terminal.enabled" ${term.enabled ? 'checked' : ''}>
          <span>Terminale abilitato</span>
        </label>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-term-host">Host SSH</label>
          <input type="text" id="sc-${key}-term-host" data-field="terminal.host" value="${escapeHtml(term.host || '')}" placeholder="host.example.com" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-term-port">Porta</label>
          <input type="number" id="sc-${key}-term-port" data-field="terminal.port" min="1" max="65535" value="${term.port || 22}">
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-term-user">Utente</label>
          <input type="text" id="sc-${key}-term-user" data-field="terminal.user" value="${escapeHtml(term.user || '')}" placeholder="root" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-term-auth">Autenticazione</label>
          <select id="sc-${key}-term-auth" data-field="terminal.auth">
            ${TERM_AUTHS.map(([v, l]) => `<option value="${v}"${term.auth === v ? ' selected' : ''}>${l}</option>`).join('')}
          </select>
        </div>
      </div>
      <div class="form-row" data-term-fields="password">
        <div class="form-group">
          <label for="sc-${key}-term-passenv">Env var password</label>
          <input type="text" id="sc-${key}-term-passenv" data-field="terminal.password_env" value="${escapeHtml(term.password_env || '')}" placeholder="MY_SSH_PASSWORD" autocomplete="off">
        </div>
        <div class="form-group"></div>
      </div>
      <div class="form-row" data-term-fields="key">
        <div class="form-group">
          <label for="sc-${key}-term-keypath">Percorso chiave privata</label>
          <input type="text" id="sc-${key}-term-keypath" data-field="terminal.key_path" value="${escapeHtml(term.key_path || '')}" placeholder="/home/user/.ssh/id_ed25519" autocomplete="off">
        </div>
        <div class="form-group">
          <label for="sc-${key}-term-dir">Cartella locale</label>
          <input type="text" id="sc-${key}-term-dir" data-field="terminal.dir" value="${escapeHtml(term.dir || '')}" placeholder="~" autocomplete="off">
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label for="sc-${key}-term-split">Layout</label>
          <select id="sc-${key}-term-split" data-field="terminal.split">
            <option value="v"${term.split === 'h' ? '' : ' selected'}>Verticale (pagina sopra, terminale sotto)</option>
            <option value="h"${term.split === 'h' ? ' selected' : ''}>Orizzontale (pagina a sinistra, terminale a destra)</option>
          </select>
        </div>
        <div class="form-group"></div>
      </div>
    `
  }

  attachServiceListEvents(list) {
    list.querySelectorAll('.edit-tab').forEach(btn => {
      btn.addEventListener('click', () => this.startEdit(btn.dataset.tabId))
    })
    list.querySelectorAll('.cancel-edit-btn').forEach(btn => {
      btn.addEventListener('click', () => this.cancelEdit())
    })
    list.querySelectorAll('.cancel-new-btn').forEach(btn => {
      btn.addEventListener('click', () => this.cancelEdit())
    })
    list.querySelectorAll('.save-row-btn').forEach(btn => {
      btn.addEventListener('click', () => this.saveRow(btn.dataset.tabId))
    })
    list.querySelectorAll('.save-new-btn').forEach(btn => {
      btn.addEventListener('click', () => this.saveNewRow())
    })

    // zoom buttons/ranges (live update of the % label; saved with the row)
    list.querySelectorAll('.zoom-out, .zoom-in').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.preventDefault()
        const id = btn.dataset.tabId
        const range = list.querySelector(`.zoom-range[data-tab-id="${id}"]`)
        const delta = btn.classList.contains('zoom-in') ? 10 : -10
        if (range) {
          range.value = Math.max(50, Math.min(250, Number(range.value) + delta))
          this.updateZoomLabel(id, range.value)
        }
      })
    })
    list.querySelectorAll('.zoom-reset').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.preventDefault()
        const id = btn.dataset.tabId
        const range = list.querySelector(`.zoom-range[data-tab-id="${id}"]`)
        if (range) {
          range.value = 100
          this.updateZoomLabel(id, 100)
        }
      })
    })
    list.querySelectorAll('.zoom-range').forEach(range => {
      range.addEventListener('input', () => this.updateZoomLabel(range.dataset.tabId, range.value))
    })

    list.querySelectorAll('.remove-tab').forEach(btn => {
      btn.addEventListener('click', () => {
        this.removeCandidateId = String(btn.dataset.tabId)
        this.renderServiceList()
      })
    })
    list.querySelectorAll('.remove-confirm-yes').forEach(btn => {
      btn.addEventListener('click', () => {
        this.removeCandidateId = null
        this.confirmRemove(btn.dataset.tabId)
      })
    })
    list.querySelectorAll('.remove-confirm-no').forEach(btn => {
      btn.addEventListener('click', () => {
        this.removeCandidateId = null
        this.renderServiceList()
      })
    })
    list.querySelectorAll('.tab-move-up').forEach(btn => {
      btn.addEventListener('click', () => this.moveTab(btn.dataset.tabId, -1))
    })
    list.querySelectorAll('.tab-move-down').forEach(btn => {
      btn.addEventListener('click', () => this.moveTab(btn.dataset.tabId, 1))
    })
    list.querySelectorAll('.tab-item').forEach(item => {
      this.attachDragListeners(item)
    })
  }

  updateZoomLabel(tabId, value) {
    const el = this.overlay.querySelector(`#zoom-value-${tabId}`)
    if (el) el.textContent = `${value}%`
  }

  attachDragListeners(item) {
    const tabId = item.dataset.tabId
    if (tabId === NEW_ID) return

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
      this.renderServiceList()
      if (this.onReorder) this.onReorder(ids)
    })
  }

  moveTab(tabId, direction) {
    const idx = this.tabs.findIndex(t => String(t.id) === String(tabId))
    const target = idx + direction
    if (idx === -1 || target < 0 || target >= this.tabs.length) return
    const [moved] = this.tabs.splice(idx, 1)
    this.tabs.splice(target, 0, moved)
    this.renderServiceList()
    if (this.onReorder) this.onReorder(this.tabs.map(t => t.id))
  }

  // ── Edit / save / remove ─────────────────────────────

  startEdit(tabId) {
    const tab = this.tabs.find(t => String(t.id) === String(tabId))
    if (!tab) return
    this.editingId = tabId
    this.renderServiceList()
  }

  cancelEdit() {
    this.editingId = null
    this.removeCandidateId = null
    this.renderServiceList()
  }

  startNewTab() {
    this.editingId = NEW_ID
    this.renderServiceList()
  }

  async saveRow(tabId) {
    this.hideError()
    const list = this.overlay.querySelector('#service-list')
    const panel = list.querySelector(`.service-edit-panel[data-tab-id="${tabId}"]`)
    if (!panel) return
    const tab = this.tabs.find(t => String(t.id) === String(tabId))
    if (!tab) return
    const svc = this.serviceForTab(tab)
    const svcKey = this.serviceKeyForTab(tab)

    const label = panel.querySelector('#tabf-label-' + tabId)?.value?.trim() || ''
    if (!label) {
      this.showError('Inserisci un etichetta.')
      return
    }
    const iconName = panel.querySelector('#tabf-icon-' + tabId)?.value || 'server'
    const url = panel.querySelector('#tabf-url-' + tabId)?.value?.trim() || tab.url || ''

    try {
      await this.onUpdateTab(tabId, { label, icon: iconName, url })

      if (svc && svcKey && this.onSaveService) {
        const patch = this.readServicePatch(panel, svc)
        await this.onSaveService(svcKey, patch)
      }

      const zoomRange = panel.querySelector('.zoom-range')
      if (zoomRange) {
        const zoom = Number(zoomRange.value) / 100
        await this.onUpdateSettings(tabId, { zoom })
      }

      const notes = panel.querySelector('.tab-notes-input')?.value ?? ''
      if (notes !== (tab.notes || '')) {
        await this.onNotesChange(tabId, notes)
      }

      this.editingId = null
      this.showToast(`Servizio ${label} salvato`)
    } catch (error) {
      this.showError('Errore: ' + error.message)
    }
  }

  async saveNewRow() {
    this.hideError()
    const list = this.overlay.querySelector('#service-list')
    const panel = list.querySelector(`.service-edit-panel[data-tab-id="${NEW_ID}"]`)
    if (!panel) return
    const label = panel.querySelector('#tabf-label-' + NEW_ID)?.value?.trim() || ''
    const url = panel.querySelector('#tabf-url-' + NEW_ID)?.value?.trim() || ''
    const iconName = panel.querySelector('#tabf-icon-' + NEW_ID)?.value || 'server'

    if (!label) {
      this.showError('Inserisci un etichetta.')
      return
    }

    try {
      await this.onSave({ label, icon: iconName, url })
      this.editingId = null
      this.showToast('Servizio aggiunto')
    } catch (error) {
      this.showError('Errore: ' + error.message)
    }
  }

  readServicePatch(panel, svc) {
    const get = (sel) => panel.querySelector(sel)?.value ?? ''
    const getCheck = (sel) => panel.querySelector(sel)?.checked ?? false
    const auth = svc.auth || {}
    const term = svc.terminal || {}
    return {
      name: get('[data-field="name"]') || svc.name || '',
      base_url: get('[data-field="base_url"]'),
      backoffice_url: get('[data-field="backoffice_url"]'),
      frontend_url: get('[data-field="frontend_url"]'),
      admin_path: get('[data-field="admin_path"]'),
      api_prefix: get('[data-field="api_prefix"]') || '/api',
      timeout_seconds: Number(get('[data-field="timeout_seconds"]') || 30),
      proxy_enabled: getCheck('[data-field="proxy_enabled"]'),
      auth: {
        type: get('[data-field="auth.type"]') || 'none',
        username_env: get('[data-field="auth.username_env"]'),
        password_env: get('[data-field="auth.password_env"]'),
        token_env: get('[data-field="auth.token_env"]'),
      },
      terminal: {
        enabled: getCheck('[data-field="terminal.enabled"]'),
        host: get('[data-field="terminal.host"]'),
        port: Number(get('[data-field="terminal.port"]') || 22),
        user: get('[data-field="terminal.user"]'),
        auth: get('[data-field="terminal.auth"]') || 'agent',
        password_env: get('[data-field="terminal.password_env"]'),
        key_path: get('[data-field="terminal.key_path"]'),
        dir: get('[data-field="terminal.dir"]'),
        split: get('[data-field="terminal.split"]') || 'v',
      },
    }
  }

  async confirmRemove(tabId) {
    try {
      await this.onRemoveTab(tabId)
      this.tabs = this.tabs.filter(t => String(t.id) !== String(tabId))
      this.renderServiceList()
    } catch (error) {
      this.showError('Errore: ' + error.message)
    }
  }

  // ── Feedback helpers ─────────────────────────────────

  showToast(message) {
    const box = this.overlay.querySelector('#modal-error')
    if (box) {
      box.textContent = message
      box.classList.add('toast')
      box.hidden = false
      clearTimeout(this._toastTimer)
      this._toastTimer = setTimeout(() => { box.hidden = true }, 2200)
    }
  }

  showError(message) {
    const box = this.overlay.querySelector('#modal-error')
    if (!box) return
    box.textContent = message
    box.classList.remove('toast')
    box.hidden = false
  }

  hideError() {
    const box = this.overlay.querySelector('#modal-error')
    if (box) box.hidden = true
  }
}
