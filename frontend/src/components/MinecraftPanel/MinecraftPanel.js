import { icon, formatTime, statusBadge } from '../Shared/utils.js'
import { PanelBase } from '../Shared/PanelBase.js'
import { api } from '../../services/api.js'

export class MinecraftPanel extends PanelBase {
  constructor() {
    super({
      iconName: 'server',
      title: 'Minecraft Network',
      panelClass: 'minecraft-panel',
      bindRefresh: () => api.getMinecraftData(),
    })
  }

  contentHtml() {
    if (!this.data) return '<div class="empty-state">Nessun dato disponibile</div>'

    const { servers = [], players = [], status = {} } = this.data
    const onlineServers = servers.filter(s => s.status === 'online').length

    return `
      <div class="dashboard-grid">
        <section class="card status-card">
          <div class="card-header">
            <h3>${icon('activity', 18)} Stato Network</h3>
            ${statusBadge(status.healthy !== false ? 'healthy' : 'unhealthy', status.healthy !== false)}
          </div>
          <div class="status-metrics">
            <div class="metric-group">
              <div class="metric">
                <span class="metric-label">Server Online</span>
                <span class="metric-value large">${onlineServers}/${servers.length}</span>
              </div>
              <div class="metric">
                <span class="metric-label">Giocatori Totali</span>
                <span class="metric-value large">${players.reduce((sum, p) => sum + (p.online ? 1 : 0), 0)}</span>
              </div>
            </div>
            <div class="health-details">
              ${Object.entries(status).filter(([k]) => !['healthy'].includes(k)).map(([key, value]) => `
                <div class="metric">
                  <span class="metric-label">${key}</span>
                  <span class="metric-value">${JSON.stringify(value)}</span>
                </div>
              `).join('')}
            </div>
          </div>
        </section>

        <section class="card servers-card">
          <div class="card-header">
            <h3>${icon('server', 18)} Server (${servers.length})</h3>
          </div>
          <div class="table-container">
            ${servers.length > 0 ? `
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Nome</th>
                    <th>Indirizzo</th>
                    <th>Stato</th>
                    <th>Giocatori</th>
                    <th>Versione</th>
                    <th>MOTD</th>
                  </tr>
                </thead>
                <tbody>
                  ${servers.map(s => `
                    <tr>
                      <td><strong>${s.name}</strong></td>
                      <td><code>${s.address}</code></td>
                      <td>${statusBadge(s.status)}</td>
                      <td>${s.online}/${s.maxPlayers}</td>
                      <td>${s.version || '-'}</td>
                      <td class="motd-cell">${s.motd || '-'}</td>
                    </tr>
                  `).join('')}
                </tbody>
              </table>
            ` : '<div class="empty-state">Nessun server configurato</div>'}
          </div>
        </section>

        <section class="card players-card">
          <div class="card-header">
            <h3>${icon('gamepad', 18)} Giocatori Online (${players.filter(p => p.online).length})</h3>
          </div>
          <div class="table-container">
            ${players.length > 0 ? `
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Nome</th>
                    <th>Server</th>
                    <th>Stato</th>
                    <th>Ultimo Accesso</th>
                  </tr>
                </thead>
                <tbody>
                  ${players.filter(p => p.online).map(p => `
                    <tr>
                      <td><strong>${p.name}</strong></td>
                      <td>${p.server}</td>
                      <td>${statusBadge('online', true)}</td>
                      <td>${p.lastSeen ? formatTime(p.lastSeen) : 'Ora'}</td>
                    </tr>
                  `).join('')}
                  ${players.filter(p => !p.online).slice(0, 10).map(p => `
                    <tr class="offline">
                      <td>${p.name}</td>
                      <td>${p.server}</td>
                      <td>${statusBadge('offline', false)}</td>
                      <td>${p.lastSeen ? formatTime(p.lastSeen) : '-'}</td>
                    </tr>
                  `).join('')}
                </tbody>
              </table>
            ` : '<div class="empty-state">Nessun giocatore registrato</div>'}
          </div>
        </section>
      </div>
    `
  }
}