import { icon, formatTime, formatNumber, statusBadge } from '../Shared/utils.js'
import { PanelBase } from '../Shared/PanelBase.js'
import { api } from '../../services/api.js'

export class SlotBuilderPanel extends PanelBase {
  constructor() {
    super({
      iconName: 'gamepad',
      title: 'SlotBuilder',
      panelClass: 'slotbuilder-panel',
      bindRefresh: () => api.getSlotBuilderData(),
      refreshMs: 60000,
    })
  }

  contentHtml() {
    if (!this.data) return '<div class="empty-state">Nessun dato disponibile</div>'

    const { games = [], analytics = [], deployments = [] } = this.data

    return `
      <div class="dashboard-grid">
        <section class="card games-card">
          <div class="card-header">
            <h3>${icon('gamepad', 18)} Giochi (${games.length})</h3>
          </div>
          <div class="games-grid">
            ${games.length > 0 ? games.map(g => `
              <article class="game-card">
                <div class="game-header">
                  <h4>${g.name}</h4>
                  ${statusBadge(g.status)}
                </div>
                <div class="game-meta">
                  <div class="meta-item">
                    <span class="meta-label">Versione</span>
                    <span class="meta-value">${g.version || '-'}</span>
                  </div>
                  <div class="meta-item">
                    <span class="meta-label">RTP</span>
                    <span class="meta-value">${g.rtp ? (g.rtp * 100).toFixed(2) + '%' : '-'}</span>
                  </div>
                  <div class="meta-item">
                    <span class="meta-label">Volatilità</span>
                    <span class="meta-value">${g.volatility || '-'}</span>
                  </div>
                  <div class="meta-item">
                    <span class="meta-label">Ultimo Deploy</span>
                    <span class="meta-value">${g.lastDeploy ? formatTime(g.lastDeploy) : '-'}</span>
                  </div>
                </div>
              </article>
            `).join('') : '<div class="empty-state">Nessun gioco configurato</div>'}
          </div>
        </section>

        <section class="card deployments-card">
          <div class="card-header">
            <h3>${icon('rocket', 18)} Deploy Recenti (${deployments.length})</h3>
          </div>
          <div class="table-container">
            ${deployments.length > 0 ? `
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Gioco</th>
                    <th>Ambiente</th>
                    <th>Versione</th>
                    <th>Stato</th>
                    <th>Data</th>
                    <th>Deployato da</th>
                  </tr>
                </thead>
                <tbody>
                  ${deployments.slice(0, 10).map(d => `
                    <tr>
                      <td>${d.gameId}</td>
                      <td><span class="env-badge env-${d.environment.toLowerCase()}">${d.environment}</span></td>
                      <td>${d.version}</td>
                      <td>${statusBadge(d.status)}</td>
                      <td>${formatTime(d.deployedAt)}</td>
                      <td>${d.deployedBy}</td>
                    </tr>
                  `).join('')}
                </tbody>
              </table>
            ` : '<div class="empty-state">Nessun deploy recente</div>'}
          </div>
        </section>

        <section class="card analytics-card">
          <div class="card-header">
            <h3>${icon('chart', 18)} Analytics Oggi</h3>
          </div>
          <div class="analytics-summary">
            ${analytics.length > 0 ? `
              <div class="analytics-grid">
                ${analytics.slice(0, 6).map(a => `
                  <div class="analytics-item">
                    <div class="analytics-game">${a.gameId}</div>
                    <div class="analytics-metrics">
                      <div class="metric">
                        <span class="metric-label">Spin</span>
                        <span class="metric-value">${formatNumber(a.spins)}</span>
                      </div>
                      <div class="metric">
                        <span class="metric-label">Bet Totale</span>
                        <span class="metric-value">€${formatNumber(a.totalBet.toFixed(0))}</span>
                      </div>
                      <div class="metric">
                        <span class="metric-label">Win Totale</span>
                        <span class="metric-value">€${formatNumber(a.totalWin.toFixed(0))}</span>
                      </div>
                      <div class="metric">
                        <span class="metric-label">RTP</span>
                        <span class="metric-value">${(a.rtp * 100).toFixed(2)}%</span>
                      </div>
                      <div class="metric">
                        <span class="metric-label">Giocatori</span>
                        <span class="metric-value">${formatNumber(a.uniquePlayers)}</span>
                      </div>
                    </div>
                  </div>
                `).join('')}
              </div>
            ` : '<div class="empty-state">Nessun dato analytics per oggi</div>'}
          </div>
        </section>
      </div>
    `
  }
}