import { createApp } from './app.js'
import './styles/main.css'

function showFatalError(message) {
  const app = document.getElementById('app')
  if (!app) return
  app.innerHTML = `
    <div style="height:100%;display:flex;align-items:center;justify-content:center;flex-direction:column;gap:1rem;color:#f7768e;font-family:Inter,sans-serif;padding:2rem;text-align:center;">
      <h2>Errore di inizializzazione</h2>
      <p>${String(message)}</p>
    </div>
  `
}

async function initApp() {
  const loading = document.getElementById('loading')
  if (loading) loading.remove()

  try {
    await createApp()
    console.log('App initialized successfully')
  } catch (e) {
    console.error('Failed to initialize app:', e)
    showFatalError(e && e.message ? e.message : e)
  }
}

window.addEventListener('error', (e) => {
  console.error('Global error:', e)
})

window.addEventListener('unhandledrejection', (e) => {
  console.error('Unhandled rejection:', e)
})

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initApp)
} else {
  initApp()
}