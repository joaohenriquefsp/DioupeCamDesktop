package domain

// FrameWriter recebe frames BGRA decodificados e os envia para a saída (ex: webcam virtual).
type FrameWriter interface {
	WriteFrame(bgra []byte)
	Close()
}

// StreamSource conecta-se à fonte de vídeo e produz frames BGRA via callback.
type StreamSource interface {
	Start(onFrame func([]byte)) error
	Stop()
}
