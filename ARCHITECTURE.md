# DioupeCamDesktop — Arquitetura

## Visão geral

App Windows que expõe a câmera do celular (Android) como **webcam virtual** reconhecida por qualquer software (Teams, Zoom, OBS, Unity). Roda na bandeja do sistema, sem janela permanente.

## Stack atual (v1 — bandeja)

| Camada | Tecnologia | Motivo |
|---|---|---|
| Linguagem | Go 1.22+ | Binário único, baixa memória, ótimo I/O |
| UI | System tray (`fyne.io/systray`) | Sem janela — app de fundo |
| Recepção H.264 | `net.Conn` TCP (stdlib Go) | Zero dependências, latência mínima |
| Decode H.264 | FFmpeg subprocess (stdin→stdout) | Melhor decoder disponível |
| Câmera virtual | Unity Capture (shared memory Windows) | DirectShow — reconhecido por qualquer app |
| Windows APIs | `golang.org/x/sys/windows` | CreateFileMapping, MapViewOfFile |

## Stack planejada (v2 — interface completa)

| Camada | Tecnologia |
|---|---|
| Backend | Go (código atual reaproveitado 100%) |
| Frontend | React + Tailwind CSS |
| Bridge Go ↔ React | Wails v2 |
| Janela | WebView2 (Edge embutido no Windows 10/11) |
| Binário final | ~10MB |

A migração para Wails não altera nenhum arquivo Go existente — apenas adiciona bindings que expõem as funções ao React.

## Fluxo de dados

```
DioupeCam (Android)
  └─ TCP :8554
        │
        ▼
   H264Client.connect()
   auto-detect: USB (localhost) → WiFi (IP configurado)
        │
        ▼
   net.Conn → io.Copy → FFmpeg stdin
                              │
                         FFmpeg decode H.264
                              │
                         BGRA raw → stdout
                              │
                    App lê frames BGRA
                              │
               UnityCaptureWriter.WriteFrame()
                              │
                Windows Named Shared Memory
                "UnityCapture_0"
                              │
              Unity Capture DirectShow filter
                              │
              qualquer app vê como webcam
```

## Estrutura de arquivos

```
DioupeCamDesktop/
  main.go              — App struct (Start/Stop), entry point
  tray.go              — Bandeja: ícone ICO gerado em memória, menu, status
  config.go            — Config JSON em %APPDATA%\DioupeCamDesktop\config.json
  h264_client.go       — TCP + FFmpeg subprocess + leitura de frames BGRA
  unity_capture.go     — Windows shared memory → Unity Capture DirectShow
  go.mod
  ARCHITECTURE.md      — este arquivo
```

## Conexão USB vs WiFi

| Modo | Como ativar | Latência |
|---|---|---|
| USB (recomendado) | `adb forward tcp:8554 tcp:8554` | ~5–15ms |
| WiFi | Configurar IP no config.json | ~30–60ms |

Auto-detect: tenta `localhost:8554` com timeout 500ms. Se falhar, usa o IP configurado.

## Protocolo Unity Capture (shared memory)

**Nome:** `UnityCapture_0`

| Offset | Tipo | Conteúdo |
|---|---|---|
| 0 | uint32 | Largura (pixels) |
| 4 | uint32 | Altura (pixels) |
| 8 | uint32 | Formato: `0` = BGRA32 |
| 12 | uint32 | Flags (reservado, `0`) |
| 16 | []byte | Pixels BGRA row-major top-down |

## Ícone da bandeja

Gerado em memória no formato ICO completo (ICONDIR + ICONDIRENTRY + BITMAPINFOHEADER + pixels BGRA + máscara AND). Necessário porque o `fyne.io/systray` no Windows passa os bytes diretamente para `CreateIconFromResourceEx`, que exige o formato ICO nativo — não PNG.

## Roadmap

- [x] Backend Go completo (H264, FFmpeg, Unity Capture, tray)
- [x] Ícone da bandeja funcionando
- [ ] Instalar FFmpeg e Unity Capture na máquina
- [ ] Testar stream ponta a ponta com Android
- [ ] Migrar para Wails v2 + React (interface completa)
- [ ] Preview de câmera em tempo real na UI
- [ ] Configurações via interface (sem editar JSON)
- [ ] Auto-update
