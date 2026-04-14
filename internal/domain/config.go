package domain

type Config struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func DefaultConfig() Config {
	return Config{
		IP:     "192.168.0.100",
		Port:   8554,
		Width:  1280,
		Height: 720,
	}
}
