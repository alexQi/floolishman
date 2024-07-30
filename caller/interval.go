package caller

import (
	"time"
)

type CallerInterval struct {
	CallerCommon
}

func (c *CallerInterval) Start() {
	go func() {
		for {
			select {
			// 定时查询数据是否满足开仓条件
			case <-time.After(CheckOpenInterval * time.Second):
				for _, option := range c.pairOptions {
					c.EventCallOpen(option.Pair)
				}
			}
		}
	}()
	c.Listen()
}
