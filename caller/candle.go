package caller

import "floolishman/model"

type CallerCandle struct {
	CallerBase
}

func (c *CallerCandle) Start(options map[string]model.PairOption, setting CallerSetting) {
	c.pairOptions = options
	c.setting = setting
}
