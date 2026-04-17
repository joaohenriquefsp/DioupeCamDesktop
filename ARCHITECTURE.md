# DioupeCamDesktop — Arquitetura

## Visão geral

App Windows com UI (Wails v2) que expõe a câmera do celular Android como **webcam virtual** reconhecida por qualquer software (Discord, Teams, Zoom, OBS). Recebe H.264 raw via TCP, decodifica com FFmpeg e escreve frames RGBA na shared memory do Unity Capture (DirectShow source filter).

## Stack

| Camada | Tecnologia | Motivo |
|---|---|---|
| Linguagem backend | Go 1.22+ | Binário único, baixa memória, ótimo I/O |
| UI frontend | React + TypeScript (Vite) | Interface configurável com preview ao vivo |
| Framework desktop | Wails v2 | Bridge Go ↔ React via WebView2 |
| Recepção H.264 | `net.Conn` TCP (stdlib Go) | Zero dependências, latência mínima |
| Decode H.264 | FFmpeg subprocess (stdin→stdout RGBA) | Melhor decoder disponível |
| Câmera virtual | Unity Capture (Windows named shared memory) | DirectShow — reconhecido por qualquer app |
| Windows APIs | `golang.org/x/sys/windows` | CreateFileMapping, MapViewOfFile, eventos |

## Fluxo de dados

```
DioupeCam (Android)
  └─ TCP :8554
        │
        ▼
   H264Client.connect()
   auto-detect: USB (localhost:8554 via adb forward) → WiFi (IP:8554)
        │
        ▼
   net.Conn → io.Copy → FFmpeg stdin
                              │
                         FFmpeg decode H.264
                         filter: crop last row + scale + pad black + setsar
                              │
                         RGBA raw → stdout (pix_fmt rgba)
                              │
                    io.ReadFull → frame completo
                              │
              ┌───────────────┴────────────────┐
              │                                 │
  UnityCaptureWriter.WriteFrame()         fpsCounter.Add(1)
  (shared memory + mutex + SetEvent)      enc.push(rgba, w, h)
              │                                 │
  Unity Capture DirectShow          encodePreviewJPEG (half-res, q80)
              │                                 │
  qualquer app vê como webcam        EventsEmit("frame", base64)
                                               │
                                      React <img> (preview ao vivo)
```

## Auto-reconexão

Quando o stream cai inesperadamente (celular sai do WiFi, app Android fecha etc.):

1. Goroutine de leitura do FFmpeg detecta o erro e chama `onStreamEnd(err)`
2. `onStreamEnd` para o pipeline (`core.Stop()`) e emite `"reconnecting"` para o React
3. `reconnectLoop` tenta `Connect()` a cada 3 segundos
4. Ao reconectar, emite `"connected"` → React atualiza o estado
5. Se o usuário clicar **Desconectar**, `manualDisconnect=true` e o loop para

## Estrutura de arquivos

```
DioupeCamDesktop/
  main.go                          — entry point Wails
  app.go                           — App struct: Connect/Disconnect/GetConfig/SaveConfig
                                     auto-reconexão, FPS goroutine, previewEncoder
  wails.json                       — config Wails (nome, info)
  frontend/
    src/App.tsx                    — UI React: sidebar config + preview + badge FPS
    src/App.css                    — Dark theme, grid 280px+1fr, responsivo 640px
  internal/
    domain/
      config.go                   — struct Config {IP, Port, Width, Height}
      stream.go                   — interfaces StreamSource, FrameWriter
    app/
      app.go                      — orquestra Start/Stop do pipeline
    infrastructure/
      capture/
        unity_capture.go          — UnityCaptureWriter (shared memory + mutex + events)
      network/
        h264_client.go            — H264Client: TCP connect + FFmpeg subprocess + ReadFull
                                    onDone callback para detectar queda do stream
      config/
        repository.go             — Load/Save JSON em %APPDATA%\DioupeCamDesktop\
  go.mod / go.sum
```

## Conexão USB vs WiFi

| Modo | Como ativar | Latência |
|---|---|---|
| USB (recomendado) | `adb forward tcp:8554 tcp:8554` | ~5–15ms |
| WiFi | Configurar IP na UI | ~30–60ms |

Auto-detect: tenta `localhost:8554` com timeout 500ms. Se falhar, usa o IP configurado.

## Protocolo Unity Capture (shared memory)

Baseado em `shared.inl` do github.com/schellingb/UnityCapture.

**Nomes dos objetos Windows** (CapNum=0):
- Shared memory: `"UnityCapture_Data"`
- Mutex: `"UnityCapture_Mutx"`
- Evento frame pronto: `"UnityCapture_Sent"`
- Evento quer frame: `"UnityCapture_Want"`

**Header (32 bytes):**

| Offset | Tipo | Campo | Valor |
|---|---|---|---|
| 0 | uint32 | maxSize | `width * height * 4` |
| 4 | int32 | width | largura em pixels |
| 8 | int32 | height | altura em pixels |
| 12 | int32 | stride | `width` em **pixels** (não bytes) |
| 16 | int32 | format | `0` = FORMAT_UINT8 (RGBA) |
| 20–28 | int32 | resize/mirror/timeout | `0 / 0 / 1000` |

Pixels após o header: RGBA row-major top-down.

**Formato:** Unity Capture recebe RGBA e converte para BGRA internamente via `RGBA8toBGRA8()`. FFmpeg gera `-pix_fmt rgba`.

## FFmpeg filter chain

```
crop=in_w:in_h-1:0:0
  → remove última linha (encoder MediaCodec HW deixa chroma não inicializado → verde)
scale=W:H:force_original_aspect_ratio=decrease
  → escala preservando aspect ratio
pad=W:H:(ow-iw)/2:(oh-ih)/2:color=black
  → padding preto nas bordas (sem color explícito → verde em YUV)
setsar=1
```

## Eventos Wails (Go → React)

| Evento | Payload | Quando |
|---|---|---|
| `"frame"` | `string` (JPEG base64) | A cada ~25fps durante stream ativo |
| `"fps"` | `number` | A cada segundo durante stream ativo |
| `"reconnecting"` | `number` (tentativa) | Ao detectar queda; `0` = primeira notificação |
| `"connected"` | — | Auto-reconexão bem-sucedida |

## Roadmap

### Concluído
- [x] Pipeline completo: Android H.264 TCP → FFmpeg → RGBA → Unity Capture
- [x] Câmera virtual funcionando em Discord, Teams, OBS
- [x] UI Wails v2 + React com preview ao vivo (~25fps)
- [x] Linha verde corrigida (MediaCodec last row + pad color=black)
- [x] Animação Lottie no placeholder de desconectado
- [x] Auto-detect USB/WiFi
- [x] **Auto-reconexão** com loop de retry a cada 3s
- [x] **Indicador de FPS** em tempo real na UI

### Pendente
- [ ] Renomear câmera virtual para "DioupeCam" (requer filtro DirectShow customizado — `FiltroCamUnity/DioupeFilter`)
- [ ] Installer NSIS — empacota app + Unity Capture + FFmpeg num único `.exe`
- [ ] Play Store — publicar DioupeCam Android (package `com.dioupe.camstream`)
