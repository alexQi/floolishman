package service

import (
	"context"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils"
	"reflect"
	"sync"
)

type ServiceProcessor struct {
	mu         sync.Mutex
	ctx        context.Context
	strategy   types.CompositesStrategy
	dataframes map[string]map[string]*model.Dataframe

	// 对外暴露
	pairOptions map[string]model.PairOption
	Samples     map[string]map[string]map[string]*model.Dataframe
	RealCandles map[string]map[string]*model.Candle
	PairPrices  map[string]float64
}

func NewServiceProcessor(ctx context.Context, strategy types.CompositesStrategy) *ServiceProcessor {
	return &ServiceProcessor{
		ctx:         ctx,
		dataframes:  make(map[string]map[string]*model.Dataframe),
		Samples:     make(map[string]map[string]map[string]*model.Dataframe),
		RealCandles: make(map[string]map[string]*model.Candle),
		PairPrices:  make(map[string]float64),
		pairOptions: make(map[string]model.PairOption),
		strategy:    strategy,
	}
}

func (sp *ServiceProcessor) SetPairDataframe(option model.PairOption) {
	sp.pairOptions[option.Pair] = option
	sp.PairPrices[option.Pair] = 0
	if sp.dataframes[option.Pair] == nil {
		sp.dataframes[option.Pair] = make(map[string]*model.Dataframe)
	}
	if sp.Samples[option.Pair] == nil {
		sp.Samples[option.Pair] = make(map[string]map[string]*model.Dataframe)
	}
	if sp.RealCandles[option.Pair] == nil {
		sp.RealCandles[option.Pair] = make(map[string]*model.Candle)
	}
	// 初始化不同时间周期的dataframe 及 samples
	for _, strategy := range sp.strategy.Strategies {
		sp.dataframes[option.Pair][strategy.Timeframe()] = &model.Dataframe{
			Pair:     option.Pair,
			Metadata: make(map[string]model.Series[float64]),
		}
		if _, ok := sp.Samples[option.Pair][strategy.Timeframe()]; !ok {
			sp.Samples[option.Pair][strategy.Timeframe()] = make(map[string]*model.Dataframe)
		}
		sp.Samples[option.Pair][strategy.Timeframe()][reflect.TypeOf(strategy).Elem().Name()] = &model.Dataframe{
			Pair:     option.Pair,
			Metadata: make(map[string]model.Series[float64]),
		}
	}
}

func (sp *ServiceProcessor) setDataFrame(dataframe model.Dataframe, candle model.Candle) model.Dataframe {
	if len(dataframe.Time) > 0 && candle.Time.Equal(dataframe.Time[len(dataframe.Time)-1]) {
		last := len(dataframe.Time) - 1
		dataframe.Close[last] = candle.Close
		dataframe.Open[last] = candle.Open
		dataframe.High[last] = candle.High
		dataframe.Low[last] = candle.Low
		dataframe.Volume[last] = candle.Volume
		dataframe.Time[last] = candle.Time
		for k, v := range candle.Metadata {
			dataframe.Metadata[k][last] = v
		}
	} else {
		dataframe.Close = append(dataframe.Close, candle.Close)
		dataframe.Open = append(dataframe.Open, candle.Open)
		dataframe.High = append(dataframe.High, candle.High)
		dataframe.Low = append(dataframe.Low, candle.Low)
		dataframe.Volume = append(dataframe.Volume, candle.Volume)
		dataframe.Time = append(dataframe.Time, candle.Time)
		dataframe.LastUpdate = candle.Time
		for k, v := range candle.Metadata {
			dataframe.Metadata[k] = append(dataframe.Metadata[k], v)
		}
	}
	return dataframe
}

func (sp *ServiceProcessor) updateDataFrame(timeframe string, candle model.Candle) {
	tempDataframe := sp.setDataFrame(*sp.dataframes[candle.Pair][timeframe], candle)
	sp.dataframes[candle.Pair][timeframe] = &tempDataframe
}

func (sp *ServiceProcessor) OnRealCandle(timeframe string, candle model.Candle) {
	sp.mu.Lock()         // 加锁
	defer sp.mu.Unlock() // 解锁
	oldCandle, ok := sp.RealCandles[candle.Pair][timeframe]
	if ok && oldCandle.UpdatedAt.Before(candle.UpdatedAt) == false {
		return
	}
	sp.RealCandles[candle.Pair][timeframe] = &candle
	sp.PairPrices[candle.Pair] = candle.Close
	// 采样数据转换指标
	for _, str := range sp.strategy.Strategies {
		if len(sp.dataframes[candle.Pair][timeframe].Close) < str.WarmupPeriod() {
			continue
		}
		// 执行数据采样
		sample := sp.dataframes[candle.Pair][timeframe].Sample(str.WarmupPeriod())
		// 加入最新指标
		sample = sp.setDataFrame(sample, candle)
		str.Indicators(&sample)
		// 在向Samples添加之前，确保对应的键存在
		if timeframe == str.Timeframe() {
			sp.Samples[candle.Pair][timeframe][reflect.TypeOf(str).Elem().Name()] = &sample
		}
	}
}

func (sp *ServiceProcessor) OnCandle(timeframe string, candle model.Candle) {
	if len(sp.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Before(sp.dataframes[candle.Pair][timeframe].Time[len(sp.dataframes[candle.Pair][timeframe].Time)-1]) {
		utils.Log.Errorf("late candle received: %#v", candle)
		return
	}
	// 更新Dataframe
	sp.updateDataFrame(timeframe, candle)
	sp.OnRealCandle(timeframe, candle)
}
