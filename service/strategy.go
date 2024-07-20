package service

import (
	"context"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"github.com/adshao/go-binance/v2/futures"
	"reflect"
	"sync"
	"time"
)

type StrategySetting struct {
	CheckMode            string
	FollowSymbol         bool
	LossTimeDuration     int
	FullSpaceRadio       float64
	BaseLossRatio        float64
	ProfitableScale      float64
	InitProfitRatioLimit float64
}

type ServiceStrategy struct {
	ctx                  context.Context
	strategy             types.CompositesStrategy
	dataframes           map[string]map[string]*model.Dataframe
	samples              map[string]map[string]map[string]*model.Dataframe
	lastUpdate           time.Time
	realCandles          map[string]map[string]*model.Candle
	pairPrices           map[string]float64
	pairOptions          map[string]model.PairOption
	broker               reference.Broker
	exchange             reference.Exchange
	guider               *ServiceGuider
	started              bool
	backtest             bool
	checkMode            string
	followSymbol         bool
	fullSpaceRadio       float64
	lossTimeDuration     int
	baseLossRatio        float64
	profitableScale      float64
	initProfitRatioLimit float64
	profitRatioLimit     map[string]float64
	lossLimitTimes       map[string]time.Time
	mu                   sync.Mutex
	// 仓位检查员
	positionJudgers map[string]*types.PositionJudger
}

var (
	CancelLimitDuration   time.Duration = 60
	CheckOpenInterval     time.Duration = 10
	CheckCloseInterval    time.Duration = 500
	CheckTimeoutInterval  time.Duration = 500
	CheckStrategyInterval time.Duration = 2
	ResetStrategyInterval time.Duration = 120
	StopLossDistanceRatio float64       = 0.9
	OpenPassCountLimit                  = 10
)

func NewServiceStrategy(
	ctx context.Context,
	strategySetting StrategySetting,
	strategy types.CompositesStrategy,
	broker reference.Broker,
	exchange reference.Exchange,
	guider *ServiceGuider,
	backtest bool,
) *ServiceStrategy {
	return &ServiceStrategy{
		ctx:                  ctx,
		exchange:             exchange,
		dataframes:           make(map[string]map[string]*model.Dataframe),
		samples:              make(map[string]map[string]map[string]*model.Dataframe),
		realCandles:          make(map[string]map[string]*model.Candle),
		pairPrices:           make(map[string]float64),
		pairOptions:          make(map[string]model.PairOption),
		strategy:             strategy,
		broker:               broker,
		backtest:             backtest,
		guider:               guider,
		checkMode:            strategySetting.CheckMode,
		followSymbol:         strategySetting.FollowSymbol,
		fullSpaceRadio:       strategySetting.FullSpaceRadio,
		lossTimeDuration:     strategySetting.LossTimeDuration,
		baseLossRatio:        strategySetting.BaseLossRatio,
		profitableScale:      strategySetting.ProfitableScale,
		initProfitRatioLimit: strategySetting.InitProfitRatioLimit,
		profitRatioLimit:     make(map[string]float64),
		lossLimitTimes:       make(map[string]time.Time),
		positionJudgers:      make(map[string]*types.PositionJudger),
	}
}

func (s *ServiceStrategy) Start() {
	s.started = true
	switch s.checkMode {
	case "frequency":
		for _, option := range s.pairOptions {
			go s.StartJudger(option.Pair)
		}
		break
	case "interval":
		go s.TickerCheckForOpen(s.pairOptions)
		break
	case "watchdog":
		go s.WatchdogCall(s.pairOptions)
		break
	default:
		utils.Log.Infof("Default mode Candle will processed")
	}
	// 监听仓位关闭信号重置judger
	go s.RegisterOrderSignal()
	// 非回溯测试时，执行检查仓位关闭
	if s.backtest == false {
		// 执行超时检查
		go s.TickerCheckForTimeout()
		// 非回溯测试模式且不是看门狗方式下监听平仓
		if s.followSymbol == false {
			go s.TickerCheckForClose(s.pairOptions)
		}
	}
}

func (s *ServiceStrategy) RegisterOrderSignal() {
	for {
		select {
		case orderClose := <-types.OrderCloseChan:
			s.ResetJudger(orderClose.Pair)
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

// WatchdogCall 看门狗模式下不需要知道当前要开那个仓位，
func (s *ServiceStrategy) WatchdogCall(options map[string]model.PairOption) {
	tickerCheck := time.NewTicker(CheckStrategyInterval * time.Second)
	tickerClose := time.NewTicker(CheckCloseInterval * time.Millisecond)
	for {
		select {
		case <-tickerCheck.C:
			userPositions, err := s.guider.FetchPosition()
			if err != nil {
				return
			}
			if len(userPositions) == 0 {
				utils.Log.Infof("[Watchdog] guider has not open any postion")
			}
			// 跟随模式下，开仓平仓都跟随看门口
			if s.followSymbol {
				for _, userPosition := range userPositions {
					if len(userPosition) > 1 {
						continue
					}
					if _, ok := userPosition[model.PositionSideTypeLong]; !ok {
						s.openPositionForWatchdog(userPosition[model.PositionSideTypeShort][0])
					}
					if _, ok := userPosition[model.PositionSideTypeShort]; !ok {
						s.openPositionForWatchdog(userPosition[model.PositionSideTypeLong][0])
					}
				}
			} else {
				var longShortRatio float64
				for _, option := range options {
					if _, ok := userPositions[option.Pair]; !ok {
						continue
					}
					if _, ok := userPositions[option.Pair][model.PositionSideTypeLong]; !ok {
						longShortRatio = 0
					}
					if _, ok := userPositions[option.Pair][model.PositionSideTypeShort]; !ok {
						longShortRatio = 1
					}
					if len(userPositions[option.Pair][model.PositionSideTypeLong]) == len(userPositions[option.Pair][model.PositionSideTypeShort]) {
						continue
					}
					if len(userPositions[option.Pair][model.PositionSideTypeLong]) > len(userPositions[option.Pair][model.PositionSideTypeShort]) {
						longShortRatio = 1
					} else {
						longShortRatio = 0
					}
					assetPosition, quotePosition, err := s.broker.Position(option.Pair)
					if err != nil {
						utils.Log.Error(err)
					}
					s.openPosition(option, assetPosition, quotePosition, longShortRatio, map[string]int{"watchdog": 1})
				}
			}
		case <-tickerClose.C:
			if s.followSymbol {
				s.closePostionForWatchdog()
			}
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (s *ServiceStrategy) EventCall(pair string) {
	if s.started {
		assetPosition, quotePosition, longShortRatio, matcherStrategy := s.checkPosition(s.pairOptions[pair])
		if longShortRatio >= 0 {
			s.openPosition(s.pairOptions[pair], assetPosition, quotePosition, longShortRatio, matcherStrategy)
		}
	}
}

func (s *ServiceStrategy) TickerCheckForOpen(options map[string]model.PairOption) {
	for {
		select {
		// 定时查询数据是否满足开仓条件
		case <-time.After(CheckOpenInterval * time.Second):
			for _, option := range options {
				s.EventCall(option.Pair)
			}
		}
	}
}

func (s *ServiceStrategy) TickerCheckForClose(options map[string]model.PairOption) {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckCloseInterval * time.Millisecond):
			for _, option := range options {
				s.closeOption(option)
			}
		}
	}
}

func (s *ServiceStrategy) TickerCheckForTimeout() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			s.timeoutOption()
		}
	}
}

func (s *ServiceStrategy) StartJudger(pair string) {
	tickerCheck := time.NewTicker(CheckStrategyInterval * time.Second)
	tickerReset := time.NewTicker(ResetStrategyInterval * time.Second)
	for {
		select {
		case <-tickerCheck.C:
			// 执行策略
			finalTendency := s.Process(pair)
			// 获取多空比
			longShortRatio, matcherStrategy := s.getStrategyLongShortRatio(finalTendency, s.positionJudgers[pair].Matchers)
			if s.backtest == false && len(s.positionJudgers[pair].Matchers) > 0 {
				utils.Log.Infof(
					"[JUDGE] Pair: %s | LongShortRatio: %.2f | TendencyCount: %v | MatcherStrategy:【%v】",
					pair,
					longShortRatio,
					s.positionJudgers[pair].TendencyCount,
					matcherStrategy,
				)
			}
			// 加权因子计算复合策略的趋势判断待调研是否游泳 todo
			// 多空比不满足开仓条件
			if longShortRatio < 0 {
				continue
			}
			// 计算当前方向通过总数
			passCount := 0
			for _, i := range matcherStrategy {
				passCount += i
			}
			// 当前方向通过次数少于阈值 不开仓
			if passCount < OpenPassCountLimit {
				continue
			}
			// 执行开仓检查
			assetPosition, quotePosition, err := s.broker.Position(pair)
			if err != nil {
				utils.Log.Error(err)
			}
			s.openPosition(s.pairOptions[pair], assetPosition, quotePosition, longShortRatio, matcherStrategy)
		case <-tickerReset.C:
			utils.Log.Infof("[JUDGE RESET] Pair: %s | TendencyCount: %v", pair, s.positionJudgers[pair].TendencyCount)
			s.ResetJudger(pair)
		}
	}
}

func (s *ServiceStrategy) ResetJudger(pair string) {
	s.positionJudgers[pair] = &types.PositionJudger{
		Pair:          pair,
		Matchers:      []types.StrategyPosition{},
		TendencyCount: make(map[string]int),
		Count:         0,
		CreatedAt:     time.Now(),
	}
}

func (s *ServiceStrategy) Process(pair string) string {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	// 如果 pair 在 positionJudgers 中不存在，则初始化
	if _, ok := s.positionJudgers[pair]; !ok {
		s.ResetJudger(pair)
	}
	// 执行计数器+1
	s.positionJudgers[pair].Count++
	// 执行策略检查
	matchers := s.strategy.CallMatchers(s.samples[pair])
	// 清洗策略结果
	finalTendency, currentMatchers := s.Sanitizer(matchers)
	// 重组匹配策略数据
	s.positionJudgers[pair].Matchers = append(s.positionJudgers[pair].Matchers, currentMatchers...)
	// 更新趋势计数
	s.positionJudgers[pair].TendencyCount[finalTendency]++
	// 返回当前趋势
	return finalTendency
}

func (s *ServiceStrategy) checkPosition(option model.PairOption) (float64, float64, float64, map[string]int) {
	if _, ok := s.realCandles[option.Pair]; !ok {
		return 0, 0, -1, map[string]int{}
	}
	matchers := s.strategy.CallMatchers(s.samples[option.Pair])
	finalTendency, currentMatchers := s.Sanitizer(matchers)
	if len(currentMatchers) > 1 {
		fmt.Printf("%s", len(currentMatchers))
	}
	longShortRatio, matcherStrategy := s.getStrategyLongShortRatio(finalTendency, currentMatchers)
	// 判断策略结果
	if s.backtest == true && len(currentMatchers) > 0 {
		utils.Log.Infof(
			"[JUDGE] Tendency: %s | Pair: %s | LongShortRatio: %.2f | Matchers:【%v】",
			finalTendency,
			option.Pair,
			longShortRatio,
			matcherStrategy,
		)
	}
	if longShortRatio < 0 {
		return 0, 0, longShortRatio, matcherStrategy
	}
	assetPosition, quotePosition, err := s.broker.Position(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return 0, 0, longShortRatio, matcherStrategy
	}
	return assetPosition, quotePosition, longShortRatio, matcherStrategy
}

func (s *ServiceStrategy) openPositionForWatchdog(guiderPosition model.GuiderPosition) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	if _, ok := s.pairOptions[guiderPosition.Symbol]; !ok {
		return
	}
	// 判断当前资产
	assetPosition, quotePosition, err := s.broker.Position(guiderPosition.Symbol)
	if err != nil {
		utils.Log.Error(err)
	}
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	// 当前仓位为多，最近策略为多，保持仓位
	if assetPosition > 0 && model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeLong {
		return
	}
	// 当前仓位为空，最近策略为空，保持仓位
	if assetPosition < 0 && model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeShort {
		return
	}
	// 获取当前交易对配置
	config, err := s.guider.FetchPairConfig(guiderPosition.PortfolioId, guiderPosition.Symbol)
	if err != nil {
		return
	}
	currentPrice := s.pairPrices[guiderPosition.Symbol]
	amount := (quotePosition * float64(config.Leverage) * (guiderPosition.InitialMargin / guiderPosition.AvailQuote)) / currentPrice

	var finalSide model.SideType
	if model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeLong {
		finalSide = model.SideTypeBuy
		if currentPrice > guiderPosition.EntryPrice {
			profitRatio := calc.ProfitRatio(finalSide, guiderPosition.EntryPrice, currentPrice, float64(config.Leverage), amount)
			if profitRatio > 0.12/100*float64(config.Leverage) {
				return
			}
		}
	} else {
		finalSide = model.SideTypeSell
		if currentPrice < guiderPosition.EntryPrice {
			profitRatio := calc.ProfitRatio(finalSide, guiderPosition.EntryPrice, currentPrice, float64(config.Leverage), amount)
			if profitRatio > 0.12/100*float64(config.Leverage) {
				return
			}
		}
	}
	openedPositions, err := s.broker.GetPositionsForPair(guiderPosition.Symbol)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var existPosition *model.Position
	for _, openedPosition := range openedPositions {
		if openedPosition.PositionSide == guiderPosition.PositionSide {
			existPosition = openedPosition
			break
		}
	}
	// 当前方向已存在仓位，不在开仓
	if existPosition.ID > 0 {
		if s.backtest == false {
			utils.Log.Infof(
				"[WATCHDOG HOLD ORDER - %s] Pair: %s | Price: %v | Quantity: %v  | Side: %s |  OrderFlag: %s",
				existPosition.Status,
				existPosition.Pair,
				existPosition.AvgPrice,
				existPosition.Quantity,
				existPosition.Side,
				existPosition.OrderFlag,
			)
		}
		return
	}

	// 设置当前交易对信息
	err = s.exchange.SetPairOption(s.ctx, model.PairOption{
		Pair:       config.Symbol,
		Leverage:   config.Leverage,
		MarginType: futures.MarginType(config.MarginType),
	})
	if err != nil {
		return
	}
	utils.Log.Infof(
		"[OPEN WATCHDOG] Pair: %s | Price: %v | Quantity: %v | Side: %s",
		guiderPosition.Symbol,
		currentPrice,
		amount,
		finalSide,
	)
	guiderPositionRate := calc.FormatFloatRate(amount / guiderPosition.PositionAmount)
	// 根据最新价格创建限价单
	_, err = s.broker.CreateOrderLimit(finalSide, model.PositionSideType(guiderPosition.PositionSide), guiderPosition.Symbol, amount, currentPrice, model.OrderExtra{
		Leverage:           config.Leverage,
		GuiderPositionRate: guiderPositionRate,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
}

func (s *ServiceStrategy) closePostionForWatchdog() {
	userPositions, err := s.guider.FetchPosition()
	if err != nil {
		return
	}
	openedPositions, err := s.broker.GetPositionsForOpened()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var tempSideType model.SideType
	var currentGuiderPositionRate, currentQuantity float64
	// 循环当前仓位，根据仓位orderFlag查询当前仓位关联的所有订单
	for _, openedPosition := range openedPositions {
		// 获取平仓方向
		if openedPosition.PositionSide == string(model.PositionSideTypeLong) {
			tempSideType = model.SideTypeSell
		} else {
			tempSideType = model.SideTypeBuy
		}
		// 判断当前币种仓位是否在guider中存在，平掉全部仓位
		if _, ok := userPositions[openedPosition.Pair]; !ok {
			_, err := s.broker.CreateOrderMarket(
				tempSideType,
				model.PositionSideType(openedPosition.PositionSide),
				openedPosition.Pair,
				openedPosition.Quantity,
				model.OrderExtra{
					OrderFlag:      openedPosition.OrderFlag,
					Leverage:       openedPosition.Leverage,
					LongShortRatio: openedPosition.LongShortRatio,
					MatchStrategy:  openedPosition.MatchStrategy,
				},
			)
			if err != nil {
				utils.Log.Error(err)
				return
			}
		} else {
			currentGuiderPositions, ok := userPositions[openedPosition.Pair][model.PositionSideType(openedPosition.PositionSide)]
			// 判断用户当前仓位是否在guider的仓位中，不在则继续平仓
			if !ok {
				_, err := s.broker.CreateOrderMarket(
					tempSideType,
					model.PositionSideType(openedPosition.PositionSide),
					openedPosition.Pair,
					openedPosition.Quantity,
					model.OrderExtra{
						OrderFlag:      openedPosition.OrderFlag,
						Leverage:       openedPosition.Leverage,
						LongShortRatio: openedPosition.LongShortRatio,
						MatchStrategy:  openedPosition.MatchStrategy,
					},
				)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			} else {
				// 双方都持有相同方向的仓位，判断仓位缩放比例
				currentGuiderPositionRate = calc.FormatFloatRate(openedPosition.Quantity / currentGuiderPositions[0].PositionAmount)
				// 仓位存在，判断当前仓位比例是否和之前一致,一致时跳过
				if currentGuiderPositionRate == openedPosition.GuiderPositionRate {
					continue
				}
				currentQuantity = currentGuiderPositions[0].PositionAmount * currentGuiderPositionRate
				// 判断当前计算的仓位与现在的仓位大小
				if currentQuantity < openedPosition.Quantity {
					_, err := s.broker.CreateOrderMarket(
						tempSideType,
						model.PositionSideType(openedPosition.PositionSide),
						openedPosition.Pair,
						openedPosition.Quantity-currentQuantity,
						model.OrderExtra{
							Leverage:  openedPosition.Leverage,
							OrderFlag: openedPosition.OrderFlag,
						},
					)
					if err != nil {
						utils.Log.Error(err)
						return
					}
				} else {
					// 加仓操作
					_, err := s.broker.CreateOrderMarket(
						model.SideType(openedPosition.Side),
						model.PositionSideType(openedPosition.PositionSide),
						openedPosition.Pair,
						currentQuantity-openedPosition.Quantity,
						model.OrderExtra{
							Leverage:  openedPosition.Leverage,
							OrderFlag: openedPosition.OrderFlag,
						},
					)
					if err != nil {
						utils.Log.Error(err)
						return
					}
				}
			}
		}
	}
}

// openPosition 开仓方法
func (s *ServiceStrategy) openPosition(option model.PairOption, assetPosition, quotePosition, longShortRatio float64, matcherStrategy map[string]int) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	var finalSide model.SideType
	var tempSideType model.SideType
	var postionSide model.PositionSideType

	if longShortRatio > 0.5 {
		finalSide = model.SideTypeBuy
		tempSideType = model.SideTypeSell
		postionSide = model.PositionSideTypeLong

	} else {
		finalSide = model.SideTypeSell
		tempSideType = model.SideTypeBuy
		postionSide = model.PositionSideTypeShort
	}
	// 当前仓位为多，最近策略为多，保持仓位
	if assetPosition > 0 && finalSide == model.SideTypeBuy {
		return
	}
	// 当前仓位为空，最近策略为空，保持仓位
	if assetPosition < 0 && finalSide == model.SideTypeSell {
		return
	}

	openedPositions, err := s.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	currentPrice := s.pairPrices[option.Pair]
	var existPosition *model.Position
	var reversePosition *model.Position
	for _, openedPosition := range openedPositions {
		if model.PositionSideType(openedPosition.PositionSide) == postionSide {
			existPosition = openedPosition
		} else {
			reversePosition = openedPosition
		}
	}
	// 当前方向已存在仓位，不在开仓
	if existPosition.ID > 0 {
		if s.backtest == false {
			utils.Log.Infof(
				"[WATCHDOG HOLD ORDER - %s] Pair: %s | Price: %v | Quantity: %v  | Side: %s |  OrderFlag: %s",
				existPosition.Status,
				existPosition.Pair,
				existPosition.AvgPrice,
				existPosition.Quantity,
				existPosition.Side,
				existPosition.OrderFlag,
			)
		}
		return
	}
	// 策略通过，判断当前是否已有未成交的限价单
	// 判断之前是否已有未成交的限价单
	// 直接获取当前交易对订单
	// 原始为空 止损为多  当前为多
	// 判断当前是否已有限价止损单
	// 有限价止损单时，判断止损方向和当前方向一致说明反向了
	// 在判断新的多空比和开仓多空比的大小，新的多空比绝对值比旧的小，需要继续持仓
	// 反之取消所有的限价止损单
	// ----------------

	// 与当前方向相反有仓位,计算相对分界线距离，多空比达到反手标准平仓
	if reversePosition.ID > 0 && calc.Abs(0.5-longShortRatio) >= calc.Abs(0.5-reversePosition.LongShortRatio) {
		// 判断仓位方向为反方向，平掉现有仓位
		_, err := s.broker.CreateOrderMarket(
			tempSideType,
			model.PositionSideType(reversePosition.PositionSide),
			option.Pair,
			reversePosition.Quantity,
			model.OrderExtra{
				Leverage:       option.Leverage,
				OrderFlag:      reversePosition.OrderFlag,
				LongShortRatio: reversePosition.LongShortRatio,
				MatchStrategy:  reversePosition.MatchStrategy,
			},
		)
		if err != nil {
			utils.Log.Error(err)
			return
		}
		// 删除止损时间限制配置
		delete(s.lossLimitTimes, reversePosition.OrderFlag)
		utils.Log.Infof(
			"[REVERSE POSITION - %s ] Pair: %s | Price Open: %v, Close: %v | Quantity: %v |  OrderFlag: %s",
			reversePosition.PositionSide,
			option.Pair,
			reversePosition.AvgPrice,
			currentPrice,
			reversePosition.Quantity,
			reversePosition.OrderFlag,
		)
		// 查询当前orderFlag所有的止损单，全部取消

		lossOrders, err := s.broker.GetOrdersForPostionLossUnfilled(reversePosition.OrderFlag)
		if err != nil {
			utils.Log.Error(err)
			return
		}
		for _, lossOrder := range lossOrders {
			// 取消之前的止损单
			err = s.broker.Cancel(*lossOrder)
			if err != nil {
				utils.Log.Error(err)
				return
			}
		}
	}
	// ******************* 执行反手开仓操作 *****************//
	// 根据多空比动态计算仓位大小
	scoreRadio := calc.Abs(0.5-longShortRatio) / 0.5
	amount := calc.OpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPrice, scoreRadio, s.fullSpaceRadio)
	if s.backtest == false {
		utils.Log.Infof(
			"[OPEN POSITION] Pair: %s | Price: %v | Quantity: %v | Side: %s",
			option.Pair,
			currentPrice,
			amount,
			finalSide,
		)
	}

	// 重置当前交易对止损比例
	s.profitRatioLimit[option.Pair] = 0
	// 获取最新仓位positionSide
	if finalSide == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
	} else {
		postionSide = model.PositionSideTypeShort
	}
	// 根据最新价格创建限价单
	order, err := s.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPrice, model.OrderExtra{
		Leverage:       option.Leverage,
		LongShortRatio: longShortRatio,
		MatchStrategy:  matcherStrategy,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 设置止损订单
	var stopLimitPrice float64
	var stopTrigerPrice float64

	var lossRatio = s.baseLossRatio * float64(option.Leverage)
	if scoreRadio < 0.5 {
		lossRatio = lossRatio * 0.5
	} else {
		lossRatio = lossRatio * scoreRadio
	}
	// 计算止损距离
	stopLossDistance := calc.StopLossDistance(lossRatio, order.Price, float64(s.pairOptions[option.Pair].Leverage), amount)
	if finalSide == model.SideTypeBuy {
		tempSideType = model.SideTypeSell
		stopLimitPrice = order.Price - stopLossDistance
		stopTrigerPrice = order.Price - stopLossDistance*StopLossDistanceRatio
	} else {
		tempSideType = model.SideTypeBuy
		stopLimitPrice = order.Price + stopLossDistance
		stopTrigerPrice = order.Price + stopLossDistance*StopLossDistanceRatio
	}
	_, err = s.broker.CreateOrderStopLimit(
		tempSideType,
		postionSide,
		option.Pair,
		order.Quantity,
		stopLimitPrice,
		stopTrigerPrice,
		model.OrderExtra{
			Leverage:       option.Leverage,
			OrderFlag:      order.OrderFlag,
			LongShortRatio: longShortRatio,
			MatchStrategy:  order.MatchStrategy,
		},
	)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 重置开仓检查条件
	s.ResetJudger(option.Pair)
}

func (s *ServiceStrategy) closeOption(option model.PairOption) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	// 设置时区
	loc, err := time.LoadLocation("Asia/Shanghai")
	// 获取当前已存在的仓位
	openedPositions, err := s.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) == 0 {
		return
	}
	var tempSideType model.SideType
	var currentTime time.Time
	var currentPrice float64
	var stopLossDistance float64
	var stopLimitPrice float64
	for _, openedPosition := range openedPositions {
		// 获取当前时间使用
		currentTime = time.Now()
		if s.checkMode == "candle" {
			currentTime = s.lastUpdate
		}
		currentPrice = s.pairPrices[option.Pair]
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		if _, ok := s.lossLimitTimes[openedPosition.OrderFlag]; !ok {
			s.lossLimitTimes[openedPosition.OrderFlag] = openedPosition.UpdatedAt.Add(time.Duration(s.lossTimeDuration) * time.Minute)
		}
		// 记录利润比
		profitRatio := calc.ProfitRatio(
			model.SideType(openedPosition.Side),
			openedPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		if s.backtest == false {
			utils.Log.Infof(
				"[WATCH] %s Pair: %s | Price Order: %v, Current: %v | Quantity: %v | Profit Ratio: %s | Stop Loss Time: %s",
				openedPosition.UpdatedAt.In(loc).Format("2006-01-02 15:04:05"),
				option.Pair,
				openedPosition.AvgPrice,
				currentPrice,
				openedPosition.Quantity,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				s.lossLimitTimes[openedPosition.OrderFlag].In(loc).Format("2006-01-02 15:04:05"),
			)
		}
		if model.SideType(openedPosition.Side) == model.SideTypeBuy {
			tempSideType = model.SideTypeSell
		} else {
			tempSideType = model.SideTypeBuy
		}
		// 如果利润比大于预设值，则使用计算出得利润比 - 指定步进的利润比 得到新的止损利润比
		// 小于预设值，判断止损时间
		// 此处处理时间止损
		if profitRatio < s.initProfitRatioLimit || profitRatio <= (s.profitRatioLimit[option.Pair]+s.profitableScale+0.01) {
			// 时间未达到新的止损限制时间
			if currentTime.Before(s.lossLimitTimes[openedPosition.OrderFlag]) {
				continue
			}
			// 时间超过限制时间，执行时间止损 市价平单
			_, err := s.broker.CreateOrderMarket(
				tempSideType,
				model.PositionSideType(openedPosition.PositionSide),
				option.Pair,
				openedPosition.Quantity,
				model.OrderExtra{
					Leverage:       option.Leverage,
					OrderFlag:      openedPosition.OrderFlag,
					LongShortRatio: openedPosition.LongShortRatio,
					MatchStrategy:  openedPosition.MatchStrategy,
				},
			)
			if err != nil {
				// 如果重新挂限价止损失败则不在取消
				utils.Log.Error(err)
				return
			}
			utils.Log.Infof(
				"[PROFIT TIMEOUT - %s ] Pair: %s | Price Open: %v, Close: %v | Quantity: %v |  OrderFlag: %s",
				openedPosition.PositionSide,
				option.Pair,
				openedPosition.AvgPrice,
				currentPrice,
				openedPosition.Quantity,
				openedPosition.OrderFlag,
			)
			// 删除止损时间限制配置
			delete(s.lossLimitTimes, openedPosition.OrderFlag)
			// 盈利利润由开仓时统一重置 不在处理
			// 取消所有的市价止损单
			lossOrders, err := s.broker.GetOrdersForPostionLossUnfilled(openedPosition.OrderFlag)
			if err != nil {
				continue
			}
			for _, lossOrder := range lossOrders {
				// 取消之前的止损单
				err = s.broker.Cancel(*lossOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
			continue
		}
		// 盈利时更新止损终止时间
		s.lossLimitTimes[openedPosition.OrderFlag] = currentTime.Add(time.Duration(s.lossTimeDuration) * time.Minute)
		// 递增利润比
		currentLossLimitProfit := profitRatio - s.profitableScale
		// 使用新的止损利润比计算止损点数
		stopLossDistance = calc.StopLossDistance(
			currentLossLimitProfit,
			openedPosition.AvgPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		// 重新计算止损价格
		if model.SideType(openedPosition.Side) == model.SideTypeSell {
			stopLimitPrice = openedPosition.AvgPrice - stopLossDistance
		} else {
			stopLimitPrice = openedPosition.AvgPrice + stopLossDistance
		}
		if s.backtest == false {
			utils.Log.Infof(
				"[PROFIT] Pair: %s | Side: %s | Order Price: %v, Current: %v | Quantity: %v | Profit Ratio: %s | New Stop Loss: %v, %s, Time: %s",
				option.Pair,
				openedPosition.Side,
				openedPosition.AvgPrice,
				currentPrice,
				openedPosition.Quantity,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				stopLimitPrice,
				fmt.Sprintf("%.2f%%", currentLossLimitProfit*100),
				s.lossLimitTimes[openedPosition.OrderFlag].In(loc).Format("2006-01-02 15:04:05"),
			)
		}
		// 设置新的止损单
		// 使用滚动利润比保证该止损利润是递增的
		// 不再判断新的止损价格是否小于之前的止损价格
		_, err := s.broker.CreateOrderStopMarket(tempSideType, model.PositionSideType(openedPosition.PositionSide), option.Pair, openedPosition.Quantity, stopLimitPrice, model.OrderExtra{
			Leverage:  option.Leverage,
			OrderFlag: openedPosition.OrderFlag,
		})
		if err != nil {
			// 如果重新挂限价止损失败则不在取消
			utils.Log.Error(err)
			continue
		}
		s.profitRatioLimit[option.Pair] = profitRatio - s.profitableScale
		lossOrders, err := s.broker.GetOrdersForPostionLossUnfilled(openedPosition.OrderFlag)
		if err != nil {
			continue
		}
		for _, lossOrder := range lossOrders {
			// 取消之前的止损单
			err = s.broker.Cancel(*lossOrder)
			if err != nil {
				utils.Log.Error(err)
				return
			}
		}
	}
}

func (s *ServiceStrategy) timeoutOption() {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	existOrderMap, err := s.getUnfilledOrders(s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for orderFlag, existOrders := range existOrderMap {
		positionOrders, ok := existOrders["position"]
		if !ok {
			continue
		}
		for _, positionOrder := range positionOrders {
			// 获取当前时间使用
			currentTime := time.Now()
			if s.checkMode == "candle" {
				currentTime = s.lastUpdate
			}
			// 判断当前订单是未成交，未成交的订单取消
			if positionOrder.Status == model.OrderStatusTypeNew {
				// 获取挂单时间是否超长
				cancelLimitTime := positionOrder.UpdatedAt.Add(CancelLimitDuration * time.Second)
				// 判断当前时间是否在cancelLimitTime之前,在取消时间之前则不取消,防止挂单后被立马取消
				if currentTime.Before(cancelLimitTime) {
					continue
				}
				// 取消之前的未成交的限价单
				err = s.broker.Cancel(*positionOrder)
				if err != nil {
					utils.Log.Error(err)
					continue
				}
				// 取消之前的止损单
				lossLimitOrders, ok := existOrderMap[orderFlag]["lossLimit"]
				if !ok {
					continue
				}
				for _, lossLimitOrder := range lossLimitOrders {
					// 取消之前的止损单
					err = s.broker.Cancel(*lossLimitOrder)
					if err != nil {
						utils.Log.Error(err)
						return
					}
				}
			}
		}
	}
}

func (s *ServiceStrategy) getUnfilledOrders(broker reference.Broker) (map[string]map[string][]*model.Order, error) {
	// 存储当前存在的仓位和限价单
	unfilledOrders := map[string]map[string][]*model.Order{}
	positionOrders, err := broker.GetOrdersForUnfilled()
	if err != nil {
		utils.Log.Error(err)
		return unfilledOrders, err
	}
	if len(positionOrders) > 0 {
		for _, order := range positionOrders {
			if _, ok := unfilledOrders[order.OrderFlag]; !ok {
				unfilledOrders[order.OrderFlag] = make(map[string][]*model.Order)
			}
			// 获取所有开仓单子
			if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeLong) || (order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeShort) {
				if _, ok := unfilledOrders[order.OrderFlag]["position"]; !ok {
					unfilledOrders[order.OrderFlag]["position"] = []*model.Order{}
				}
				unfilledOrders[order.OrderFlag]["position"] = append(unfilledOrders[order.OrderFlag]["position"], order)
			}
			// 获取所有平仓单子
			if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeShort) || (order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeLong) {
				if _, ok := unfilledOrders[order.OrderFlag]["lossLimit"]; !ok {
					unfilledOrders[order.OrderFlag]["lossLimit"] = []*model.Order{}
				}
				unfilledOrders[order.OrderFlag]["lossLimit"] = append(unfilledOrders[order.OrderFlag]["lossLimit"], order)
			}
		}
	}
	return unfilledOrders, nil
}

func (s *ServiceStrategy) Sanitizer(matchers []types.StrategyPosition) (string, []types.StrategyPosition) {
	var finalTendency string
	// 初始化变量
	currentMatchers := []types.StrategyPosition{}
	// 调用策略执行器
	// 如果没有匹配的策略位置，直接返回空方向
	if len(matchers) == 0 {
		return finalTendency, currentMatchers
	}
	totalScore := 0
	matcherMapScore := make(map[string]int)
	// 初始化本次趋势计数器
	// 初始化多空双方map
	tendencyCounts := make(map[string]map[string]int)
	// 更新计数器和得分
	for _, pos := range matchers {
		// 计算总得分
		totalScore += pos.Score
		// 统计当前所有得分
		if _, ok := matcherMapScore[pos.StrategyName]; !ok {
			matcherMapScore[pos.StrategyName] = pos.Score
		}
		// 趋势判断 不需要判断当前是否可用
		if _, ok := tendencyCounts[pos.Tendency]; !ok {
			tendencyCounts[pos.Tendency] = make(map[string]int)
		}
		tendencyCounts[pos.Tendency][pos.StrategyName]++
		// 跳过不可用的策略
		if pos.Useable == false {
			continue
		}
		// 统计通过的策略
		currentMatchers = append(currentMatchers, pos)
	}

	currentTendency := map[string]float64{}
	// 外层循环方向
	for td, sm := range tendencyCounts {
		for sn, count := range sm {
			currentTendency[td] += float64(count) * float64(matcherMapScore[sn]) / float64(totalScore)
		}
	}
	// 获取最终趋势
	var initTendency float64
	for tendency, tc := range currentTendency {
		if tc > initTendency {
			finalTendency = tendency
			initTendency = tc
		}
	}
	// 返回结果
	return finalTendency, currentMatchers
}

func (s *ServiceStrategy) getStrategyLongShortRatio(finalTendency string, currentMatchers []types.StrategyPosition) (float64, map[string]int) {
	longShortRatio := -1.0
	totalScore := 0
	matcherMapScore := make(map[string]int)
	matcherStrategy := make(map[string]int)
	// 无检查结果
	if len(currentMatchers) == 0 || finalTendency == "ambiguity" {
		return longShortRatio, matcherStrategy
	}
	// 计算总得分
	for _, strategy := range s.strategy.Strategies {
		totalScore += strategy.SortScore()
	}
	// 初始化多空双方map
	result := map[model.SideType]map[string]int{
		model.SideTypeBuy:  make(map[string]int),
		model.SideTypeSell: make(map[string]int),
	}
	// 统计多空双方出现次数
	for _, pos := range currentMatchers {
		// 获取策略权重评分
		if _, ok := matcherMapScore[pos.StrategyName]; !ok {
			matcherMapScore[pos.StrategyName] = pos.Score
		}
		// 统计出现次数
		result[pos.Side][pos.StrategyName]++
	}
	var buyDivisor, sellDivisor float64
	// 外层循环方向
	for sideType, sm := range result {
		for sn, count := range sm {
			// 加权计算最终得分因子
			if sideType == model.SideTypeBuy {
				buyDivisor += float64(count) * float64(matcherMapScore[sn]) / float64(totalScore)
			} else {
				sellDivisor += float64(count) * float64(matcherMapScore[sn]) / float64(totalScore)
			}
		}
	}

	if buyDivisor == sellDivisor {
		longShortRatio = -1
	} else {
		if sellDivisor == 0 {
			if buyDivisor > 0 {
				longShortRatio = 1
			} else {
				longShortRatio = -1
			}
		} else {
			if buyDivisor > 0 {
				longShortRatio = buyDivisor / (buyDivisor + sellDivisor)
			} else {
				longShortRatio = 0
			}
		}
	}
	if longShortRatio < 0 {
		return longShortRatio, matcherStrategy
	} else {
		if longShortRatio > 0.5 {
			return longShortRatio, result[model.SideTypeBuy]
		} else {
			return longShortRatio, result[model.SideTypeSell]
		}
	}
}

func (s *ServiceStrategy) SetPairDataframe(option model.PairOption) {
	s.pairOptions[option.Pair] = option
	s.pairPrices[option.Pair] = 0
	s.profitRatioLimit[option.Pair] = 0
	if s.dataframes[option.Pair] == nil {
		s.dataframes[option.Pair] = make(map[string]*model.Dataframe)
	}
	if s.samples[option.Pair] == nil {
		s.samples[option.Pair] = make(map[string]map[string]*model.Dataframe)
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
		if _, ok := s.samples[option.Pair][strategy.Timeframe()]; !ok {
			s.samples[option.Pair][strategy.Timeframe()] = make(map[string]*model.Dataframe)
		}
		s.samples[option.Pair][strategy.Timeframe()][reflect.TypeOf(strategy).Elem().Name()] = &model.Dataframe{
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

func (s *ServiceStrategy) OnRealCandle(timeframe string, candle model.Candle) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	oldCandle, ok := s.realCandles[candle.Pair][timeframe]
	if ok && oldCandle.UpdatedAt.Before(candle.UpdatedAt) == false {
		return
	}
	s.realCandles[candle.Pair][timeframe] = &candle
	s.pairPrices[candle.Pair] = candle.Close
	s.lastUpdate = candle.UpdatedAt
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
			s.samples[candle.Pair][timeframe][reflect.TypeOf(str).Elem().Name()] = &sample
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
	s.OnRealCandle(timeframe, candle)
	if s.checkMode == "candle" {
		s.EventCall(candle.Pair)
	}
}

func (s *ServiceStrategy) OnCandleForBacktest(timeframe string, candle model.Candle) {
	if len(s.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Before(s.dataframes[candle.Pair][timeframe].Time[len(s.dataframes[candle.Pair][timeframe].Time)-1]) {
		utils.Log.Errorf("late candle received: %#v", candle)
		return
	}
	// 更新Dataframe
	s.updateDataFrame(timeframe, candle)
	s.OnRealCandle(timeframe, candle)
	if s.started {
		s.EventCall(candle.Pair)
		s.closeOption(s.pairOptions[candle.Pair])
		s.timeoutOption()
	}
}
