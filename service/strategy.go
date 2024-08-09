package service

import (
	"context"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"reflect"
	"strings"
	"sync"
)

type ServiceStrategy struct {
	ctx         context.Context
	strategy    types.CompositesStrategy
	dataframes  map[string]map[string]*model.Dataframe
	realCandles map[string]map[string]*model.Candle
	caller      reference.Caller
	backtest    bool
	started     bool
	checkMode   string
	mu          sync.Mutex
}

func NewServiceStrategy(ctx context.Context, checkMode string, strategy types.CompositesStrategy, caller reference.Caller, backtest bool) *ServiceStrategy {
	return &ServiceStrategy{
		ctx:         ctx,
		dataframes:  make(map[string]map[string]*model.Dataframe),
		realCandles: make(map[string]map[string]*model.Candle),
		strategy:    strategy,
		checkMode:   checkMode,
		backtest:    backtest,
		caller:      caller,
	}
}

func (s *ServiceStrategy) Start() {
	s.started = true
	utils.Log.Infof("Checking mode set: %s", strings.ToUpper(s.checkMode))
}

func (s *ServiceStrategy) SetPairDataframe(option model.PairOption) {
	s.caller.SetPair(option)
	if s.dataframes[option.Pair] == nil {
		s.dataframes[option.Pair] = make(map[string]*model.Dataframe)
	}
	if s.realCandles[option.Pair] == nil {
		s.realCandles[option.Pair] = make(map[string]*model.Candle)
	}
	// 初始化不同时间周期的dataframe 及 samples
	for _, strategy := range s.strategy.Strategies {
		s.dataframes[option.Pair][strategy.Timeframe()] = &model.Dataframe{
			Pair:     option.Pair,
			Metadata: make(map[string]model.Series[float64]),
		}
	}
}

func (s *ServiceStrategy) setDataFrame(dataframe model.Dataframe, candle model.Candle) model.Dataframe {
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

func (s *ServiceStrategy) updateDataFrame(timeframe string, candle model.Candle) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	tempDataframe := s.setDataFrame(*s.dataframes[candle.Pair][timeframe], candle)
	s.dataframes[candle.Pair][timeframe] = &tempDataframe
}

func (s *ServiceStrategy) OnRealCandle(timeframe string, candle model.Candle, isComplate bool) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	oldCandle, ok := s.realCandles[candle.Pair][timeframe]
	if ok && oldCandle.UpdatedAt.Before(candle.UpdatedAt) == false {
		return
	}
	s.realCandles[candle.Pair][timeframe] = &candle

	// 更新交易对信息
	s.caller.UpdatePairInfo(candle.Pair, candle.Close, candle.UpdatedAt)
	// 采样数据转换指标
	for _, str := range s.strategy.Strategies {
		if len(s.dataframes[candle.Pair][timeframe].Close) < str.WarmupPeriod() {
			continue
		}
		// 执行数据采样
		sample := s.dataframes[candle.Pair][timeframe].Sample(str.WarmupPeriod())
		// 加入最新指标
		sample = s.setDataFrame(sample, candle)
		str.Indicators(&sample)
		// 在向samples添加之前，确保对应的键存在
		if timeframe == str.Timeframe() {
			// 采样数据
			s.caller.SetSample(candle.Pair, timeframe, reflect.TypeOf(str).Elem().Name(), &sample)
		}
	}
	// 未开始时
	if s.started == false {
		if s.checkMode == "grid" {
			s.caller.BuildGird(candle.Pair, timeframe, false)
		}
	} else {
		if isComplate == true {
			// 回溯测试模式
			if s.backtest {
				s.caller.EventCallOpen(candle.Pair)
				s.caller.EventCallClose(candle.Pair)
				s.caller.CheckOrderTimeout()
			} else {
				if s.checkMode == "grid" {
					s.caller.BuildGird(candle.Pair, timeframe, true)
				}
				if s.checkMode == "candle" {
					s.caller.EventCallOpen(candle.Pair)
				}
			}
		}
	}
}

func (s *ServiceStrategy) OnCandle(timeframe string, candle model.Candle) {
	if len(s.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Before(s.dataframes[candle.Pair][timeframe].Time[len(s.dataframes[candle.Pair][timeframe].Time)-1]) {
		utils.Log.Errorf("late candle received: %#v", candle)
		return
	}
	// 更新Dataframe
	s.updateDataFrame(timeframe, candle)
	s.OnRealCandle(timeframe, candle, true)
}
