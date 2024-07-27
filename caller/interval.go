package caller

import (
	"floolishman/model"
	"time"
)

type CallerInterval struct {
	CallerBase
	CallerCommon
}

func (c *CallerInterval) Start(options map[string]model.PairOption, setting CallerSetting) {
	c.pairOptions = options
	c.setting = setting
}

func (c *CallerInterval) TickerCheckForOpen(options map[string]model.PairOption) {
	for {
		select {
		// 定时查询数据是否满足开仓条件
		case <-time.After(CheckOpenInterval * time.Second):
			for _, option := range options {
				c.EventCallOpen(option.Pair)
			}
		}
	}
}
