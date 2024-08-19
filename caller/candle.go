package caller

type Candle struct {
	Common
}

func (c *Candle) Start() {
	go c.Listen()
}
