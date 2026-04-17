# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status do projeto

### Concluído
- [x] Stack migrada para **Wails v2** (Go backend + React/TypeScript frontend) — substituiu systray
- [x] Clean Architecture (`internal/domain`, `internal/app`, `internal/infrastructure/...`)
- [x] UI responsiva em React — sidebar 280px + preview area, sem scroll, breakpoint 640px
- [x] Live preview via Wails events (`EventsEmit`/`EventsOn`) — JPEG base64, ~25fps, metade da resolução
- [x] FFmpeg com flags corretas: `-fps_mode passthrough` (sem `-r`/`-vsync`), `-fflags nobuffer+genpts`, `-flags low_delay`
- [x] Aspect ratio preservado: `scale=W:H:force_original_aspect_ratio=decrease,pad=...`
- [x] Protocolo Unity Capture totalmente corrigido:
  - Nomes SEM sufixo `0` (CapNum=0 usa `'\0'` — verificado no source `shared.inl`)
  - Produtor (nosso app) cria `wantEvent` — filtro abre via `OpenEventA`; sem isso `Open()` retorna false e a câmera nunca ativa
  - Mutex adquirido/liberado na mesma OS thread (`runtime.LockOSThread`)
  - `SetEvent(sentEvent)` após cada escrita
  - **stride no header = `width` em PIXELS, não bytes** — `ProcessJob` usa como offset `uint32_t*`; stride em bytes causava leitura 4× fora do buffer → crash do Discord/OBS
- [x] Formato de pixel correto: FFmpeg gera **RGBA** (Unity Capture espera RGBA e converte para BGRA internamente via `RGBA8toBGRA8()`)
- [x] Câmera virtual funcionando em Discord e OBS — feed real da câmera do celular exibido corretamente
- [x] **Linha verde corrigida** — dois problemas distintos no filtro FFmpeg (`h264_client.go`):
  - Colunas verdes nas bordas esquerda/direita: pad filter sem `color` explícito usava chroma U=0,V=0 internamente em YUV → Y=0,U=0,V=0 converte para verde em RGB; corrigido com `color=black`
  - Linha verde na borda inferior: encoder MediaCodec de hardware Android corrompe a última linha do frame H.264 (chroma não inicializado); corrigido com `crop=in_w:in_h-1:0:0` antes do scale
- [x] `go.mod` + `go.sum` com todas as dependências
- [x] `preview.go` removido — servidor MJPEG legado não utilizado; preview feito via `EventsEmit` no `app.go`
- [x] **Animação Lottie** no placeholder de desconectado (`frontend/public/webcam-animation.lottie`):
  - Pacote `@lottiefiles/dotlottie-react` instalado
  - TypeScript atualizado para `^5.0.0` (necessário para compatibilidade com `DotLottieReact`)
  - `App.tsx`: `DotLottieReact` substituiu o emoji `📷` no estado desconectado

### Pendente
- [ ] Trocar nome da câmera virtual de "Unity Video Capture" para "DioupeCam" — requer filter DirectShow customizado (tarefa futura)
- [ ] Auto-reconexão — se o celular cair/reconectar, hoje precisa clicar manualmente

### Dependência
O app Android (DioupeCam) precisa estar rodando e transmitindo H.264 na porta 8554 para o desktop funcionar.

## O que é este projeto

**DioupeCamDesktop** é um app Windows com UI (Wails v2) que expõe a câmera do celular como uma **webcam virtual** que qualquer software Windows enxerga (Discord, Teams, Zoom, OBS).

Recebe o stream H.264 raw TCP do **DioupeCam** (Android), decodifica via FFmpeg, escreve frames RGBA na memória compartilhada do **Unity Capture** (DirectShow source filter) e exibe preview ao vivo na UI.

## Pré-requisitos na máquina

| Software | Instalação | Por quê |
|---|---|---|
| **Go 1.22+** | golang.org/dl | Compilar o backend |
| **Node.js 18+** | nodejs.org | Build do frontend React |
| **Wails v2** | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` | Framework desktop |
| **FFmpeg** | ffmpeg.org → adicionar ao PATH | Decodificar H.264 |
| **Unity Capture** | github.com/schellingb/UnityCapture → `Install.bat` como admin | Driver DirectShow virtual cam |

O usuário final só precisa de FFmpeg + Unity Capture. Go, Node e Wails são só para desenvolvimento.

## Stack

| Camada | Tecnologia |
|---|---|
| Linguagem backend | Go 1.22 |
| UI frontend | React + TypeScript (Vite) |
| Framework desktop | Wails v2 |
| Recepção H.264 | `net.Conn` TCP (stdlib) |
| Decode H.264 | FFmpeg subprocess (stdin→stdout RGBA) |
| Câmera virtual | Unity Capture (Windows named shared memory) |
| Windows APIs | `golang.org/x/sys/windows` |

## Arquitetura

```
DioupeCam (Android)
  └─ TCP :8554  ←  H264Client.connect()
                      │  auto-detect: tenta USB (localhost:8554) primeiro, depois WiFi (IP:8554)
                      ↓
               net.Conn → io.Copy → FFmpeg stdin
                                         │
                                    FFmpeg decode H.264
                                         │
                                    RGBA raw → stdout  (pix_fmt rgba)
                                         │
                              io.ReadFull → frame completo
                                         │
                          UnityCaptureWriter.WriteFrame(rgba)
                         ┌───────────────┴────────────────┐
                         │                                 │
              Named Shared Memory                   previewEncoder
              (mutex + SetEvent)                   (throttle 25fps)
                         │                                 │
             Unity Capture DirectShow             encodePreviewJPEG
                         │                          (half-res, q80)
             qualquer app vê como webcam                   │
                                               EventsEmit("frame", b64)
                                                           │
                                                  React img tag (UI)
```

## WiFi vs USB

| Modo | Como ativar | Latência |
|---|---|---|
| **USB** (recomendado) | `adb forward tcp:8554 tcp:8554` | ~5–15ms |
| **WiFi** | Configurar IP na UI | ~30–60ms |

O app auto-detecta: tenta `localhost:8554` (USB) com timeout 500ms, se falhar usa o IP configurado (WiFi).

## Estrutura de arquivos

```
DioupeCamDesktop/
  main.go                                    — entry point Wails
  app.go                                     — App struct: Connect/Disconnect/GetConfig/SaveConfig
                                               previewEncoder (goroutine única, throttle 40ms)
                                               encodePreviewJPEG (half-res, RGBA→JPEG)
  preview.go                                 — MJPEG server legado (não utilizado, remover)
  wails.json                                 — config Wails (nome, info)
  frontend/
    src/App.tsx                              — UI React: sidebar config + preview live
    src/App.css                              — Dark theme, grid 280px+1fr, responsivo 640px
  internal/
    domain/
      config.go                             — struct Config {IP, Port, Width, Height}
      stream.go                             — interfaces StreamSource, FrameWriter
    app/
      app.go                                — orquestra Start/Stop do pipeline
    infrastructure/
      capture/
        unity_capture.go                    — UnityCaptureWriter (shared memory + mutex + events)
      network/
        h264_client.go                      — H264Client: TCP connect + FFmpeg subprocess + ReadFull
      config/
        repository.go                       — Load/Save JSON em %APPDATA%\DioupeCamDesktop\
  go.mod / go.sum
```

## Protocolo Unity Capture (shared memory) — IMPLEMENTAÇÃO REAL

Baseado em `shared.inl` do repositório github.com/schellingb/UnityCapture.

**Nomes dos objetos Windows** (CapNum=0 — o código C++ substitui o '0' final por `'\0'` (null), então os nomes efetivos NÃO têm sufixo):
```cpp
char CSCapNumChar = (m_CapNum ? '0' + m_CapNum : '\0'); // CapNum=0 → '\0'
char CS_NAME[] = "UnityCapture_Data0";
CS_NAME[sizeof(CS_NAME)-2] = CSCapNumChar; // substitui '0' por '\0'
// nome efetivo: "UnityCapture_Data"
```
- Shared memory: `"UnityCapture_Data"`
- Mutex: `"UnityCapture_Mutx"`
- Evento "frame pronto": `"UnityCapture_Sent"`
- Evento "quer frame": `"UnityCapture_Want"`

**Header (32 bytes):**

| Offset | Tipo | Campo | Valor |
|---|---|---|---|
| 0 | `uint32` | maxSize | `width * height * 4` (só pixels, sem header) |
| 4 | `int32` | width | largura em pixels |
| 8 | `int32` | height | altura em pixels |
| 12 | `int32` | stride | `width` (em **pixels**, não bytes — ProcessJob usa como offset `uint32_t*`) |
| 16 | `int32` | format | `0` = FORMAT_UINT8 (RGBA input) |
| 20 | `int32` | resizemode | `0` = desabilitado |
| 24 | `int32` | mirrormode | `0` = desabilitado |
| 28 | `int32` | timeout | `1000` ms |

**Pixels após o header:** RGBA row-major top-down.

**Sincronização (protocolo Send() de shared.inl):**
1. `WaitForSingleObject(mutex, INFINITE)`
2. Copia pixels para `mapAddr + 32`
3. `ReleaseMutex(mutex)`
4. `SetEvent(sentEvent)`

> **Formato:** Unity Capture recebe **RGBA** e converte para BGRA internamente via `RGBA8toBGRA8()`. O FFmpeg deve gerar `-pix_fmt rgba`, não `bgra`.

## Configuração

Arquivo: `%APPDATA%\DioupeCamDesktop\config.json`

```json
{
  "ip": "192.168.0.105",
  "port": 8554,
  "width": 1280,
  "height": 720
}
```

- Editável via UI do app (campos IP, porta, resolução)
- `SaveConfig` reconecta automaticamente se já estiver conectado

## Comandos

```bash
# Desenvolvimento (hot reload frontend + backend)
wails dev

# Build final — .exe com UI embutida
wails build

# Só o frontend
cd frontend && npm run build

# Baixar dependências Go
go mod tidy

# Testar stream antes de subir a UI
ffplay -f h264 tcp://localhost:8554          # USB
ffplay -f h264 tcp://192.168.0.105:8554      # WiFi
```

## Analogias Go ↔ .NET

| Go | .NET equivalente |
|---|---|
| `goroutine` | `Task` / `async` |
| `channel` | `Channel<T>` |
| `select { case ... }` | `Task.WhenAny` |
| `defer f()` | `using` / `IDisposable` |
| `sync.Mutex` | `lock` |
| `atomic.Bool` | `Interlocked` / `volatile bool` |
| `io.ReadFull(r, buf)` | `stream.ReadExactly(buf)` |
| `exec.Command(...)` | `Process.Start(...)` |
| `unsafe.Pointer` | `IntPtr` / `Marshal` |
| `go.mod` | `.csproj` |
