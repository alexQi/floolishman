package model

type StrategyFeature struct {
	// 仓位方向
	PositionSide string
	// 最新价格
	LastPrice float64
	// 挂单价格
	OpenPrice float64
	// Rsi限制-下限
	RsiFloor float64
	// Rsi限制-上限
	RsiUpper float64
	// 最近插针变化率限制
	LimitShadowChangeRate float64
	// 挂单限制距离
	DistanceRate float64
	// rsi值
	LastRsi float64
	PrevRsi float64
	PenuRsi float64
	// rsi极限值 abs(rsi-50)/50，区分做多做空
	LastRsiExtreme float64
	PrevRsiExtreme float64
	PenuRsiExtreme float64
	// rsi相对于上根蜡烛的差值
	LastRsiDiff float64
	PrevRsiDiff float64
	PenuRsiDiff float64
	// 量能与平均量能占比
	PrevAvgVolumeRate float64
	PenuAvgVolumeRate float64
	// 价格涨跌幅
	PrevPriceRate float64
	PenuPriceRate float64
	// 蜡烛上下影线之比
	LastShadowRate float64
	PrevShadowRate float64
	PenuShadowRate float64
	// 蜡烛蜡烛上影线变化比例
	LastUpperShadowChangeRate float64
	PrevUpperShadowChangeRate float64
	PenuUpperShadowChangeRate float64
	// 蜡烛蜡烛下影线变化比例
	LastLowerShadowChangeRate float64
	PrevLowerShadowChangeRate float64
	PenuLowerShadowChangeRate float64
	// 蜡烛上影线与蜡烛实体比例
	LastUpperPinRate float64
	PrevUpperPinRate float64
	PenuUpperPinRate float64
	// 蜡烛下影线与蜡烛实体比例
	LastLowerPinRate float64
	PrevLowerPinRate float64
	PenuLowerPinRate float64
	// Macd 差值占比
	LastMacdDiffRate float64
	PrevMacdDiffRate float64
	PenuMacdDiffRate float64
	// 振幅
	LastAmplitude float64
	PrevAmplitude float64
	PenuAmplitude float64
	// 高点或低点价格突破布林带时的比例， 突破上轨时： high/bbupper 突破下轨时，bblower/low
	LastBollingCrossRate float64
	PrevBollingCrossRate float64
	PenuBollingCrossRate float64
	// 收盘价相对于布林带上下轨价格比例 靠近上轨时，close/bbupper,靠近下轨时，bblower/close
	LastCloseCrossRate float64
	PrevCloseCrossRate float64
	PenuCloseCrossRate float64
	// 时间
	OpenAt string
}

// 返回字段映射表，作为 StrategyFeature 结构体的方法
func (sf *StrategyFeature) fieldMap() map[string]func() interface{} {
	return map[string]func() interface{}{
		"LastRsi":                   func() interface{} { return sf.LastRsi },
		"PrevRsi":                   func() interface{} { return sf.PrevRsi },
		"PenuRsi":                   func() interface{} { return sf.PenuRsi },
		"LastRsiExtreme":            func() interface{} { return sf.LastRsiExtreme },
		"PrevRsiExtreme":            func() interface{} { return sf.PrevRsiExtreme },
		"PenuRsiExtreme":            func() interface{} { return sf.PenuRsiExtreme },
		"LastRsiDiff":               func() interface{} { return sf.LastRsiDiff },
		"PrevRsiDiff":               func() interface{} { return sf.PrevRsiDiff },
		"PenuRsiDiff":               func() interface{} { return sf.PenuRsiDiff },
		"PrevAvgVolumeRate":         func() interface{} { return sf.PrevAvgVolumeRate },
		"PenuAvgVolumeRate":         func() interface{} { return sf.PenuAvgVolumeRate },
		"PrevPriceRate":             func() interface{} { return sf.PrevPriceRate },
		"PenuPriceRate":             func() interface{} { return sf.PenuPriceRate },
		"LastShadowRate":            func() interface{} { return sf.LastShadowRate },
		"PrevShadowRate":            func() interface{} { return sf.PrevShadowRate },
		"PenuShadowRate":            func() interface{} { return sf.PenuShadowRate },
		"LastUpperShadowChangeRate": func() interface{} { return sf.LastUpperShadowChangeRate },
		"PrevUpperShadowChangeRate": func() interface{} { return sf.PrevUpperShadowChangeRate },
		"PenuUpperShadowChangeRate": func() interface{} { return sf.PenuUpperShadowChangeRate },
		"LastLowerShadowChangeRate": func() interface{} { return sf.LastLowerShadowChangeRate },
		"PrevLowerShadowChangeRate": func() interface{} { return sf.PrevLowerShadowChangeRate },
		"PenuLowerShadowChangeRate": func() interface{} { return sf.PenuLowerShadowChangeRate },
		"LastUpperPinRate":          func() interface{} { return sf.LastUpperPinRate },
		"PrevUpperPinRate":          func() interface{} { return sf.PrevUpperPinRate },
		"PenuUpperPinRate":          func() interface{} { return sf.PenuUpperPinRate },
		"LastLowerPinRate":          func() interface{} { return sf.LastLowerPinRate },
		"PrevLowerPinRate":          func() interface{} { return sf.PrevLowerPinRate },
		"PenuLowerPinRate":          func() interface{} { return sf.PenuLowerPinRate },
		"LastMacdDiffRate":          func() interface{} { return sf.LastMacdDiffRate },
		"PrevMacdDiffRate":          func() interface{} { return sf.PrevMacdDiffRate },
		"PenuMacdDiffRate":          func() interface{} { return sf.PenuMacdDiffRate },
		"LastAmplitude":             func() interface{} { return sf.LastAmplitude },
		"PrevAmplitude":             func() interface{} { return sf.PrevAmplitude },
		"PenuAmplitude":             func() interface{} { return sf.PenuAmplitude },
		"LastBollingCrossRate":      func() interface{} { return sf.LastBollingCrossRate },
		"PrevBollingCrossRate":      func() interface{} { return sf.PrevBollingCrossRate },
		"PenuBollingCrossRate":      func() interface{} { return sf.PenuBollingCrossRate },
		"LastCloseCrossRate":        func() interface{} { return sf.LastCloseCrossRate },
		"PrevCloseCrossRate":        func() interface{} { return sf.PrevCloseCrossRate },
		"PenuCloseCrossRate":        func() interface{} { return sf.PenuCloseCrossRate },
	}
}

// 获取字段值的方法
func (sf *StrategyFeature) GetFeatureValue(field string) (interface{}, bool) {
	if getter, exists := sf.fieldMap()[field]; exists {
		return getter(), true
	}
	return nil, false
}
