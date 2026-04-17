//go:build windows

package capture

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// Para CapNum=0, o código C++ faz:
	//   char CS_NAME[] = "UnityCapture_Data0";
	//   CS_NAME[sizeof(CS_NAME)-2] = (m_CapNum ? '0'+m_CapNum : '\0');
	// Com CapNum=0 → '\0' substitui o '0' → nome efetivo sem sufixo.
	sharedMemName = "UnityCapture_Data"
	mutexName     = "UnityCapture_Mutx"
	sentEventName = "UnityCapture_Sent"
	wantEventName = "UnityCapture_Want"
	headerSize    = 32
	formatUINT8   = 0
)

type UnityCaptureWriter struct {
	width     uint32
	height    uint32
	handle    windows.Handle
	mapAddr   uintptr
	frameSize uint32
	mutex     windows.Handle
	sentEvent windows.Handle
	wantEvent windows.Handle

	latestMu sync.Mutex
	latest   []byte
	hasFrame bool
	notify   chan struct{}
	quit     chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
}

func New(width, height uint32) (*UnityCaptureWriter, error) {
	frameSize := width * height * 4
	totalSize := uint32(headerSize) + frameSize

	namePtr, _ := windows.UTF16PtrFromString(sharedMemName)
	handle, err := windows.CreateFileMapping(
		windows.InvalidHandle, nil, windows.PAGE_READWRITE,
		0, totalSize, namePtr,
	)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, fmt.Errorf("CreateFileMapping %q: %w — Unity Capture instalado?", sharedMemName, err)
	}

	addr, err := windows.MapViewOfFile(handle, windows.FILE_MAP_WRITE, 0, 0, uintptr(totalSize))
	if err != nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("MapViewOfFile: %w", err)
	}

	mutxPtr, _ := windows.UTF16PtrFromString(mutexName)
	mutex, err := windows.CreateMutex(nil, false, mutxPtr)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		windows.UnmapViewOfFile(addr)
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("CreateMutex %q: %w", mutexName, err)
	}

	sentPtr, _ := windows.UTF16PtrFromString(sentEventName)
	sentEvent, err := windows.CreateEvent(nil, 0, 0, sentPtr)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		windows.UnmapViewOfFile(addr)
		windows.CloseHandle(handle)
		windows.CloseHandle(mutex)
		return nil, fmt.Errorf("CreateEvent %q: %w", sentEventName, err)
	}

	// wantEvent: criado pelo produtor para que o filtro possa sinalizá-lo.
	// Algumas versões do filtro chamam OpenEvent(wantEvent) ao inicializar.
	wantPtr, _ := windows.UTF16PtrFromString(wantEventName)
	wantEvent, _ := windows.CreateEvent(nil, 0, 0, wantPtr)
	// wantEvent é opcional — falha não é fatal

	w := &UnityCaptureWriter{
		width:     width,
		height:    height,
		handle:    handle,
		mapAddr:   addr,
		frameSize: frameSize,
		mutex:     mutex,
		sentEvent: sentEvent,
		wantEvent: wantEvent,
		notify:    make(chan struct{}, 1),
		quit:      make(chan struct{}),
	}
	w.writeHeader()
	log.Printf("[UC] shared memory criada: %dx%d, frameSize=%d, totalSize=%d", width, height, frameSize, totalSize)
	log.Printf("[UC] handles — mem:%v mutex:%v sentEvent:%v", handle, mutex, sentEvent)

	// Diagnóstico: verifica quais events do Unity Capture existem no sistema.
	// O filtro DirectShow do Unity Capture cria wantEvent ao inicializar.
	// Se nenhum desses nomes for encontrado, o filtro não está rodando.
	diagEvents := []string{
		"UnityCapture_Want", "UnityCapture_Want0",
		"UnityCapture_Sent", "UnityCapture_Sent0",
		"Global\\UnityCapture_Want0", "Global\\UnityCapture_Sent0",
	}
	for _, n := range diagEvents {
		p, _ := windows.UTF16PtrFromString(n)
		h, err := windows.OpenEvent(windows.SYNCHRONIZE, false, p)
		if err == nil {
			log.Printf("[UC] DIAG event EXISTE: %q", n)
			windows.CloseHandle(h)
		} else {
			log.Printf("[UC] DIAG event ausente: %q", n)
		}
	}

	w.wg.Add(1)
	go w.loop()
	return w, nil
}

func (w *UnityCaptureWriter) writeHeader() {
	p := w.mapAddr
	*(*uint32)(unsafe.Pointer(p + 0)) = w.frameSize
	*(*int32)(unsafe.Pointer(p + 4)) = int32(w.width)
	*(*int32)(unsafe.Pointer(p + 8)) = int32(w.height)
	*(*int32)(unsafe.Pointer(p + 12)) = int32(w.width) // stride em PIXELS (não bytes) — ProcessJob usa como offset uint32_t*
	*(*int32)(unsafe.Pointer(p + 16)) = formatUINT8
	*(*int32)(unsafe.Pointer(p + 20)) = 0
	*(*int32)(unsafe.Pointer(p + 24)) = 0
	*(*int32)(unsafe.Pointer(p + 28)) = 1000
}

func (w *UnityCaptureWriter) loop() {
	// Windows mutex é thread-affine: WaitForSingleObject e ReleaseMutex precisam
	// rodar na MESMA OS thread. LockOSThread garante que o scheduler do Go
	// não mova esta goroutine para outra thread durante as operações de mutex.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer w.wg.Done()
	blank := make([]byte, w.frameSize)
	for i := 3; i < len(blank); i += 4 {
		blank[i] = 255 // alpha=255: preto opaco em vez de transparente
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastWrite time.Time
	heartbeatCount := 0
	realFrameCount := 0

	log.Printf("[UC] loop iniciado")

	for {
		select {
		case <-w.quit:
			log.Printf("[UC] loop encerrado (frames reais: %d, heartbeats: %d)", realFrameCount, heartbeatCount)
			return
		case <-w.notify:
			w.latestMu.Lock()
			if !w.hasFrame {
				w.latestMu.Unlock()
				continue
			}
			rgba := w.latest
			w.hasFrame = false
			w.latestMu.Unlock()
			w.doWrite(rgba)
			lastWrite = time.Now()
			realFrameCount++
			if realFrameCount == 1 {
				// Loga os primeiros 4 pixels (top-left) para diagnosticar formato
				log.Printf("[UC] frame real #1 escrito — primeiros 4 pixels:")
				for i := 0; i < 4 && (i+1)*4 <= len(rgba); i++ {
					b := rgba[i*4 : i*4+4]
					log.Printf("[UC]   pixel[%d]: byte0=%d byte1=%d byte2=%d byte3=%d", i, b[0], b[1], b[2], b[3])
				}
				// Loga pixel do centro
				mid := (w.height/2*w.width + w.width/2) * 4
				if int(mid)+4 <= len(rgba) {
					b := rgba[mid : mid+4]
					log.Printf("[UC]   pixel[centro]: byte0=%d byte1=%d byte2=%d byte3=%d", b[0], b[1], b[2], b[3])
				}
			} else if realFrameCount%300 == 0 {
				log.Printf("[UC] frame real #%d escrito", realFrameCount)
			}
		case <-ticker.C:
			if time.Since(lastWrite) > 250*time.Millisecond {
				w.doWrite(blank)
				heartbeatCount++
				if heartbeatCount == 1 || heartbeatCount%20 == 0 {
					log.Printf("[UC] heartbeat #%d (sem stream)", heartbeatCount)
				}
			}
			// Checa se o filtro sinalizou wantEvent (prova que está ativo)
			if w.wantEvent != 0 {
				if r, _ := windows.WaitForSingleObject(w.wantEvent, 0); r == 0 {
					log.Printf("[UC] wantEvent sinalizado — filtro está ATIVO e pedindo frames!")
				}
			}
		}
	}
}

// acquireMutex tenta adquirir o mutex em tentativas de 500ms.
// Retorna false se quit for sinalizado (shutdown). Nunca usa timeout fixo
// para não descartar frames durante inicialização do filtro DirectShow.
func (w *UnityCaptureWriter) acquireMutex() bool {
	attempts := 0
	for {
		r, err := windows.WaitForSingleObject(w.mutex, 500)
		if r == 0x00000000 || r == 0x00000080 { // WAIT_OBJECT_0 ou WAIT_ABANDONED
			if attempts > 0 {
				log.Printf("[UC] mutex adquirido após %d tentativa(s)", attempts+1)
			}
			return true
		}
		if r == 0xFFFFFFFF { // WAIT_FAILED
			log.Printf("[UC] WAIT_FAILED ao adquirir mutex: err=%v handle=%v", err, w.mutex)
			return false
		}
		// WAIT_TIMEOUT
		attempts++
		if attempts == 1 {
			log.Printf("[UC] mutex timeout (tentativa %d), mutex handle=%v", attempts, w.mutex)
		}
		select {
		case <-w.quit:
			return false
		default:
		}
	}
}

func (w *UnityCaptureWriter) doWrite(rgba []byte) {
	if !w.acquireMutex() {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(w.mapAddr+headerSize)), w.frameSize)
	copy(dst, rgba)
	if err := windows.ReleaseMutex(w.mutex); err != nil {
		log.Printf("[UC] ReleaseMutex falhou: %v — thread-affinity issue?", err)
	}
	if err := windows.SetEvent(w.sentEvent); err != nil {
		log.Printf("[UC] SetEvent falhou: err=%v sentEvent=%v", err, w.sentEvent)
	}
}

// WriteFrame armazena o frame mais recente e sinaliza a goroutine interna.
// Retorna imediatamente — nunca bloqueia a goroutine chamadora.
func (w *UnityCaptureWriter) WriteFrame(rgba []byte) {
	if uint32(len(rgba)) != w.frameSize {
		return
	}
	w.latestMu.Lock()
	w.latest = rgba
	w.hasFrame = true
	w.latestMu.Unlock()

	select {
	case w.notify <- struct{}{}:
	default:
	}
}

// Close para a goroutine e libera recursos. Seguro contra chamadas múltiplas.
// Aguarda a goroutine sair (no máximo ~500ms) antes de fechar os handles.
func (w *UnityCaptureWriter) Close() {
	w.once.Do(func() {
		close(w.quit)
		w.wg.Wait()
		windows.UnmapViewOfFile(w.mapAddr)
		windows.CloseHandle(w.handle)
		windows.CloseHandle(w.mutex)
		windows.CloseHandle(w.sentEvent)
		if w.wantEvent != 0 {
			windows.CloseHandle(w.wantEvent)
		}
	})
}
