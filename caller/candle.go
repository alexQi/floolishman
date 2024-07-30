package caller

type CallerCandle struct {
	CallerCommon
}

func (c *CallerCandle) Start() {
	c.Listen()
}
