import { useEffect, useRef, useState } from 'react'
import './App.css'
import { Connect, Disconnect, IsConnected, GetConfig, SaveConfig } from '../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import { DotLottieReact } from '@lottiefiles/dotlottie-react'

interface Config {
  ip: string
  port: number
  width: number
  height: number
}

const RESOLUTIONS = [
  { label: '4K — 3840×2160',   width: 3840, height: 2160 },
  { label: 'Full HD — 1920×1080', width: 1920, height: 1080 },
  { label: 'HD — 1280×720',    width: 1280, height: 720  },
  { label: '480p — 854×480',   width: 854,  height: 480  },
  { label: 'VGA — 640×480',    width: 640,  height: 480  },
]

function App() {
  const [connected, setConnected] = useState(false)
  const [config, setConfig] = useState<Config>({ ip: '', port: 8554, width: 1280, height: 720 })
  const [status, setStatus] = useState('Verificando...')
  const [loading, setLoading] = useState(false)
  const [fps, setFps] = useState(0)
  const imgRef = useRef<HTMLImageElement>(null)

  useEffect(() => {
    GetConfig().then(setConfig)
    IsConnected().then(c => {
      setConnected(c)
      setStatus(c ? 'Conectado' : 'Desconectado')
    })

    EventsOn('frame', (b64: string) => {
      if (imgRef.current && b64) {
        imgRef.current.src = 'data:image/jpeg;base64,' + b64
      }
    })

    EventsOn('fps', (f: number) => setFps(f))

    EventsOn('reconnecting', (attempt: number) => {
      setConnected(false)
      setFps(0)
      setStatus(attempt === 0 ? 'Conexão perdida...' : `Reconectando... (tentativa ${attempt})`)
    })

    EventsOn('connected', () => {
      setConnected(true)
      setStatus('Conectado')
    })

    return () => {
      EventsOff('frame')
      EventsOff('fps')
      EventsOff('reconnecting')
      EventsOff('connected')
    }
  }, [])

  async function handleConnect() {
    setLoading(true)
    setStatus('Conectando...')
    try {
      await Connect()
      setConnected(true)
      setStatus('Conectado')
    } catch (e: any) {
      setConnected(false)
      setStatus('Erro: ' + e)
    } finally {
      setLoading(false)
    }
  }

  async function handleDisconnect() {
    await Disconnect()
    setConnected(false)
    setFps(0)
    setStatus('Desconectado')
  }

  async function handleSave() {
    setLoading(true)
    try {
      await SaveConfig(config)
      setStatus(connected ? 'Config salva — reconectando...' : 'Config salva')
      const c = await IsConnected()
      setConnected(c)
      setStatus(c ? 'Conectado' : 'Desconectado')
    } catch (e: any) {
      setStatus('Erro ao salvar: ' + e)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="app">
      <header className="header">
        <div className="header-title">
          <span className="header-icon">📷</span>
          <span>DioupeCam Desktop</span>
        </div>
        <div className="header-right">
          {connected && fps > 0 && (
            <div className="badge badge-fps">{fps} fps</div>
          )}
          <div className={`badge ${connected ? 'badge-on' : 'badge-off'}`}>
            <span className="badge-dot" />
            {status}
          </div>
        </div>
      </header>

      <main className="content">
        {/* ── Sidebar ── */}
        <aside className="sidebar">
          <section className="card">
            <h2 className="card-title">Configurações</h2>

            <div className="field">
              <label>IP do celular</label>
              <input
                type="text"
                value={config.ip}
                onChange={e => setConfig({ ...config, ip: e.target.value })}
                placeholder="192.168.0.100"
              />
            </div>

            <div className="field">
              <label>Porta</label>
              <input
                type="number"
                value={config.port}
                onChange={e => setConfig({ ...config, port: +e.target.value })}
              />
            </div>

            <div className="field">
              <label>Resolução da webcam virtual</label>
              <select
                value={`${config.width}x${config.height}`}
                onChange={e => {
                  const res = RESOLUTIONS.find(r => `${r.width}x${r.height}` === e.target.value)
                  if (res) setConfig({ ...config, width: res.width, height: res.height })
                }}
              >
                {RESOLUTIONS.map(r => (
                  <option key={`${r.width}x${r.height}`} value={`${r.width}x${r.height}`}>
                    {r.label}
                  </option>
                ))}
              </select>
              <span className="field-hint">FFmpeg escala automaticamente para esta resolução</span>
            </div>

            <button className="btn btn-secondary" onClick={handleSave} disabled={loading}>
              Salvar configuração
            </button>
          </section>

          <section className="card">
            <h2 className="card-title">Conexão</h2>
            <p className="hint">
              USB: conecte o cabo e rode <code>adb forward tcp:8554 tcp:8554</code><br />
              WiFi: configure o IP do celular acima
            </p>
            {!connected ? (
              <button className="btn btn-primary" onClick={handleConnect} disabled={loading}>
                {loading ? 'Conectando...' : 'Conectar'}
              </button>
            ) : (
              <button className="btn btn-danger" onClick={handleDisconnect} disabled={loading}>
                Desconectar
              </button>
            )}
          </section>
        </aside>

        {/* ── Preview ── */}
        <div className="preview-area">
          <div className="card-preview-wrap">
            <h2 className="card-title">Preview</h2>
            {connected ? (
              <img ref={imgRef} alt="Camera preview" className="preview-img" />
            ) : (
              <div className="preview-placeholder">
                <DotLottieReact
                  src="webcam-animation.lottie"
                  loop
                  autoplay
                  style={{ width: 180, height: 180 }}
                />
                <p>Conecte para ver o preview</p>
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}

export default App
