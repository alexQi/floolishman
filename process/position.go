package process

import (
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"time"
)

var (
	CheckOpenInterval  = 30
	CheckCloseInterval = 10
)

func CheckOpenPoistion(options map[string]model.PairOption, borker reference.Broker, callback types.OpenPositionFunc) {
	for {
		select {
		// 定时查询数据是否满足开仓条件
		case <-time.After(time.Duration(CheckOpenInterval) * time.Second):
			for _, option := range options {
				callback(option, borker)
			}
		}
	}
}

func CheckClosePoistion(options map[string]model.PairOption, borker reference.Broker, callback types.ClosePositionFunc) {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(time.Duration(CheckCloseInterval) * time.Second):
			for _, option := range options {
				callback(option, borker)
			}
		}
	}
}
