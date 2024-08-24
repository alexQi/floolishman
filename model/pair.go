package model

import (
	"floolishman/constants"
	"fmt"
	"github.com/adshao/go-binance/v2/futures"
	"log"
	"strings"
)

type PairOption struct {
	Status                     bool
	Pair                       string
	Leverage                   int
	MarginType                 futures.MarginType
	MarginMode                 constants.MarginMode
	MarginSize                 float64
	MaxGridStep                float64
	MinGridStep                float64
	UndulatePriceLimit         float64
	UndulateVolumeLimit        float64
	WindowPeriod               float64
	MaxAddPosition             int64
	MinAddPosition             int64
	HoldPositionPeriod         int64
	HoldPositionPeriodDecrStep float64
	ProfitableScale            float64
	ProfitableScaleDecrStep    float64
	ProfitableTrigger          float64
	ProfitableTriggerIncrStep  float64
	PullMarginLossRatio        float64
	MaxMarginRatio             float64
	MaxMarginLossRatio         float64
	PauseCaller                int64
}

func (o PairOption) String() string {
	return fmt.Sprintf("[EXCHAGE - STATUS: %v] Loading Pair: %s, Leverage: %d, MarginType: %s", o.Status, o.Pair, o.Leverage, o.MarginType)
}

func BuildPairOption(defaultOption PairOption, valMap map[string]interface{}) PairOption {
	// 检查并处理 status
	status, ok := valMap["status"].(bool)
	if !ok {
		log.Fatalf("Invalid status format for pair %s: %v", defaultOption.Pair, valMap["status"])
	}
	defaultOption.Status = status
	// 检查并处理 leverage
	leverageFloat, ok := valMap["leverage"].(float64)
	if ok {
		defaultOption.Leverage = int(leverageFloat)
	}
	// 网格窗口期
	windowPeriod, ok := valMap["windowperiod"].(float64)
	if ok {
		defaultOption.WindowPeriod = windowPeriod
	}
	// 最大网格间隔
	maxGridStep, ok := valMap["maxgridstep"].(float64)
	if ok {
		defaultOption.MaxGridStep = maxGridStep
	}
	// 最小网格间隔
	minGridStep, ok := valMap["mingridstep"].(float64)
	if ok {
		defaultOption.MinGridStep = minGridStep
	}
	// 价格波动限制
	undulatePriceLimit, ok := valMap["undulatepricelimit"].(float64)
	if ok {
		defaultOption.UndulatePriceLimit = undulatePriceLimit
	}
	// 量能波动限制
	undulateVolumeLimit, ok := valMap["undulatevolumelimit"].(float64)
	if ok {
		defaultOption.UndulateVolumeLimit = undulateVolumeLimit
	}
	// 保证金类型
	marginType, ok := valMap["margintype"].(string)
	if ok {
		defaultOption.MarginType = futures.MarginType(strings.ToUpper(marginType))
	}
	// 保证金模式
	marginMode, ok := valMap["marginmode"].(string)
	if ok {
		defaultOption.MarginMode = constants.MarginMode(strings.ToUpper(marginMode))
	}
	// 保证金大小
	marginSize, ok := valMap["marginsize"].(float64)
	if ok {
		defaultOption.MarginSize = marginSize
	}
	// 最大加仓次数
	maxAddPosition, ok := valMap["maxaddposition"].(int)
	if ok {
		defaultOption.MaxAddPosition = int64(maxAddPosition)
	}
	// 最小加仓次数
	minAddPosition, ok := valMap["minaddposition"].(int)
	if ok {
		defaultOption.MinAddPosition = int64(minAddPosition)
	}
	// 止盈触发比例
	profitableTrigger, ok := valMap["profitabletrigger"].(float64)
	if ok {
		defaultOption.ProfitableTrigger = profitableTrigger
	}
	// 止盈触发比例-step
	profitableTriggerIncrStep, ok := valMap["profitabletriggerincrstep"].(float64)
	if ok {
		defaultOption.ProfitableTriggerIncrStep = profitableTriggerIncrStep
	}
	// 止盈触发回撤比例
	profitableScale, ok := valMap["profitablescale"].(float64)
	if ok {
		defaultOption.ProfitableScale = profitableScale
	}
	// 止盈触发回撤比例-step
	profitableScaleDecrStep, ok := valMap["profitablescaledecrstep"].(float64)
	if ok {
		defaultOption.ProfitableScaleDecrStep = profitableScaleDecrStep
	}
	// 亏损拉回比例
	pullMarginLossRatio, ok := valMap["pullmarginlossratio"].(float64)
	if ok {
		defaultOption.PullMarginLossRatio = pullMarginLossRatio
	}
	// 止盈触发回撤比例
	maxMarginRatio, ok := valMap["maxmarginratio"].(float64)
	if ok {
		defaultOption.MaxMarginRatio = maxMarginRatio
	}
	// 止盈触发回撤比例
	maxMarginLossRatio, ok := valMap["maxmarginlossratio"].(float64)
	if ok {
		defaultOption.MaxMarginLossRatio = maxMarginLossRatio
	}
	// 暂停交易时长 min
	pauseCaller, ok := valMap["pausecaller"].(int)
	if ok {
		defaultOption.PauseCaller = int64(pauseCaller)
	}
	// 持仓周期 min
	holdPositionPeriod, ok := valMap["holdpositionperiod"].(int)
	if ok {
		defaultOption.HoldPositionPeriod = int64(holdPositionPeriod)
	}
	// 持仓超越周期盈利触发百分比
	holdPositionPeriodDecrStep, ok := valMap["holdpositionperioddecrstep"].(float64)
	if ok {
		defaultOption.HoldPositionPeriodDecrStep = holdPositionPeriodDecrStep
	}

	return defaultOption
}
