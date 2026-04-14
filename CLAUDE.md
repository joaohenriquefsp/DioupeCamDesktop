# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status do projeto

### Concluído
- [x] Todos os arquivos Go implementados (`main.go`, `config.go`, `h264_client.go`, `unity_capture.go`, `tray.go`)
- [x] `go.mod` criado com dependências declaradas

### Pendente (fazer com Claude)
- [ ] **Instalar Go 1.22+** — golang.org/dl
- [ ] **Instalar FFmpeg** — ffmpeg.org → adicionar ao PATH
- [ ] **Instalar Unity Capture** — github.com/schellingb/UnityCapture → `Install.bat` como admin
- [ ] **`go mod tidy`** — baixar dependências e gerar `go.sum`
- [ ] **`go run .`** — primeiro build e teste
- [ ] **Verificar protocolo Unity Capture** — confirmar layout da shared memory em `UnityCaptureFilter.cpp` se câmera não aparecer nos apps

### Dependência
Fazer o DioupeCam (Android) funcionar primeiro — o app desktop precisa do stream H.264 na porta 8554.

## O que é este projeto

**DioupeCamDesktop** é um app Windows que roda na bandeja do sistema e expõe a câmera do celular como uma **webcam virtual** que qualquer software Windows enxerga (Teams, Zoom, OBS, Unity via `WebCamTexture`).

Recebe o stream H.264 raw TCP do **DioupeCam** (Android), decodifica via FFmpeg e escreve frames BGRA na memória compartilhada do **Unity Capture** (DirectShow source filter open source).

## Pré-requisitos na máquina

| Software | Instalação | Por quê |
|---|---|---|
| **Go 1.22+** | golang.org/dl | Compilar o app |
| **FFmpeg** | ffmpeg.org → adicionar ao PATH | Decodificar H.264 |
| **Unity Capture** | github.com/schellingb/UnityCapture → `Install.bat` como admin | Driver DirectShow virtual cam |

O usuário final só precisa de FFmpeg + Unity Capture. Go é só para desenvolvimento.

## Stack

| Camada | Tecnologia |
|---|---|
| Linguagem | Go 1.22 |
| UI | System tray (`github.com/getlantern/systray`) |
| Recepção H.264 | `net.Conn` TCP (stdlib) |
| Decode H.264 | FFmpeg subprocess (stdin→stdout) |
| Câmera virtual | Unity Capture (Windows shared memory) |
| Windows APIs | `golang.org/x/sys/windows` |

## Arquitetura

```
DioupeCam (Android)
  └─ TCP :8554  ←  H264Client.connect()
                      │  auto-detect: tenta USB (localhost) primeiro, depois WiFi (IP)
                      ↓
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
                                         │
                         Unity Capture DirectShow filter
                                         │
                         qualquer app vê como webcam
```

## WiFi vs USB

| Modo | Como ativar | Latência | Qualidade |
|---|---|---|---|
| **USB** (recomendado) | `adb forward tcp:8554 tcp:8554` | ~5–15ms | ★★★★★ |
| **WiFi** | Configurar IP no config.json | ~30–60ms | ★★★★★ |

O app **auto-detecta**: tenta `localhost:8554` (USB) com timeout de 500ms. Se falhar, usa o IP configurado (WiFi). Nenhuma configuração manual necessária quando USB estiver conectado com `adb forward` ativo.

## Estrutura de arquivos

```
DioupeCamDesktop/
  main.go            — App struct (Start/Stop), entry point
  tray.go            — Bandeja do sistema: ícone, menu, status
  config.go          — Config JSON em %APPDATA%\DioupeCamDesktop\config.json
  h264_client.go     — TCP connection + FFmpeg subprocess + leitura de frames BGRA
  unity_capture.go   — Windows shared memory → Unity Capture DirectShow filter
  go.mod
```

## Protocolo Unity Capture (shared memory)

O Go app cria o `CreateFileMapping` e o Unity Capture filter abre com `OpenFileMapping`.

**Nome da shared memory:** `"UnityCapture_0"`

| Offset | Tipo | Conteúdo |
|---|---|---|
| 0 | `uint32` | Largura (pixels) |
| 4 | `uint32` | Altura (pixels) |
| 8 | `uint32` | Formato: `0` = BGRA32 |
| 12 | `uint32` | Flags (reservado, `0`) |
| 16 | `[]byte` | Pixels BGRA row-major top-down |

> **Verificar:** confirmar header layout contra `UnityCaptureFilter.cpp` em github.com/schellingb/UnityCapture antes de testar.

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

- Editar via menu da bandeja → "Editar configuração" (abre no Notepad)
- USB não precisa alterar o IP — o app tenta localhost automaticamente

## Comandos

```bash
# Baixar dependências
go mod tidy

# Rodar em desenvolvimento (com console para ver logs)
go run .

# Build final — .exe standalone sem console (5~8MB)
go build -ldflags="-H windowsgui" -o DioupeCamDesktop.exe .

# Rodar testes
go test ./...

# Testar um arquivo específico
go test -run TestH264Client ./...
```

## Testar sem Unity Capture

Para verificar que o stream chega e decodifica corretamente antes de instalar o driver:

```bash
# Rodar em modo dev (console visível) e checar logs de "frame recebido"
go run .
```

Ou testar o stream direto com FFplay:
```bash
# WiFi
ffplay -f h264 tcp://192.168.0.105:8554

# USB (após adb forward tcp:8554 tcp:8554)
ffplay -f h264 tcp://localhost:8554
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
| `io.Copy(dst, src)` | `src.CopyToAsync(dst)` |
| `exec.Command(...)` | `Process.Start(...)` |
| `unsafe.Pointer` | `IntPtr` / `Marshal` |
| `go.mod` | `.csproj` |
