package caller

import (
	"time"
)

type Interval struct {
	Common
}

func (c *Interval) Start() {
	go func() {
		for {
			select {
			// 定时查询数据是否满足开仓条件
			case <-time.After(CheckOpenInterval * time.Second):
				for _, option := range c.pairOptions {
					if option.Status == false {
						continue
					}
					c.EventCallOpen(option.Pair)
				}
			}
		}
	}()
	go c.Listen()
}
