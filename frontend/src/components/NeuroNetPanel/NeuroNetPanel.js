import { icon, formatTime, statusBadge } from '../Shared/utils.js'
import { PanelBase } from '../Shared/PanelBase.js'
import { api } from '../../services/api.js'

export class NeuroNetPanel extends PanelBase {
  constructor() {
    super({
      iconName: 'brain',
      title: 'NeuroNet Dashboard',
      panelClass: 'neuronet-panel',
      bindRefresh: () => api.getNeuroNetData(),
    })
  }

  contentHtml() {
    if (!this.data) return '<div class="empty-state">Nessun dato disponibile</div>'

    const { models = [], training = [], health = {} } = this.data

    return `
      <div class="dashboard-grid">
        <section class="card health-card">
          <div class="card-header">
            <h3>${icon('activity', 18)} Stato Sistema</h3>
            ${statusBadge(health.status || 'unknown', health.healthy !== false)}
          </div>
          <div class="health-metrics">
            ${Object.entries(health).filter(([k]) => k !== 'status' && k !== 'healthy').map(([key, value]) => `
              <div class="metric">
                <span class="metric-label">${key}</span>
                <span class="metric-value">${JSON.stringify(value)}</span>
              </div>
            `).join('')}
          </div>
        </section>

        <section class="card models-card">
          <div class="card-header">
            <h3>${icon('database', 18)} Modelli (${models.length})</h3>
          </div>
          <div class="table-container">
            ${models.length > 0 ? `
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Nome</th>
                    <th>Versione</th>
                    <th>Stato</th>
                    <th>Accuracy</th>
                    <th>Ultimo Training</th>
                  </tr>
                </thead>
                <tbody>
                  ${models.map(m => `
                    <tr>
                      <td><strong>${m.name}</strong></td>
                      <td>${m.version || '-'}</td>
                      <td>${statusBadge(m.status)}</td>
                      <td>${m.accuracy ? (m.accuracy * 100).toFixed(2) + '%' : '-'}</td>
                      <td>${m.lastTrained ? formatTime(m.lastTrained) : '-'}</td>
                    </tr>
                  `).join('')}
                </tbody>
              </table>
            ` : '<div class="empty-state">Nessun modello configurato</div>'}
          </div>
        </section>

        <section class="card training-card">
          <div class="card-header">
            <h3>${icon('rocket', 18)} Training Jobs (${training.length})</h3>
          </div>
          <div class="table-container">
            ${training.length > 0 ? `
              <table class="data-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Modello</th>
                    <th>Stato</th>
                    <th>Progresso</th>
                    <th>Iniziato</th>
                  </tr>
                </thead>
                <tbody>
                  ${training.map(t => `
                    <tr>
                      <td><code>${t.id.slice(0, 8)}...</code></td>
                      <td>${t.modelId}</td>
                      <td>${statusBadge(t.status)}</td>
                      <td>
                        <div class="progress-bar">
                          <div class="progress-fill" style="width: ${t.progress}%"></div>
                        </div>
                        <span>${t.progress.toFixed(1)}%</span>
                      </td>
                      <td>${formatTime(t.startedAt)}</td>
                    </tr>
                  `).join('')}
                </tbody>
              </table>
            ` : '<div class="empty-state">Nessun training in corso</div>'}
          </div>
        </section>
      </div>
    `
  }
}