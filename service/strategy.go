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
	currentPirce := s.pairPrices[guiderPosition.Symbol]
	amount := (quotePosition * float64(config.Leverage) * (guiderPosition.InitialMargin / guiderPosition.AvailQuote)) / currentPirce

	var finalSide model.SideType
	if model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeLong {
		finalSide = model.SideTypeBuy
		if currentPirce > guiderPosition.EntryPrice {
			profitRatio := calc.ProfitRatio(finalSide, guiderPosition.EntryPrice, currentPirce, float64(config.Leverage), amount)
			if profitRatio > 0.12/100*float64(config.Leverage) {
				return
			}
		}
	} else {
		finalSide = model.SideTypeSell
		if currentPirce < guiderPosition.EntryPrice {
			profitRatio := calc.ProfitRatio(finalSide, guiderPosition.EntryPrice, currentPirce, float64(config.Leverage), amount)
			if profitRatio > 0.12/100*float64(config.Leverage) {
				return
			}
		}
	}
	holdedOrder := model.Order{}
	existOrderMap, err := s.getPairExistOrders(s.pairOptions[guiderPosition.Symbol], s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for _, existOrders := range existOrderMap {
		positionOrders, ok := existOrders["position"]
		if !ok {
			continue
		}
		// 获取所有仓位，包含new 和filled
		for _, positionOrder := range positionOrders {
			// 原始单方向和当前策略判断方向相同，保留原始单
			if positionOrder.Side == finalSide {
				holdedOrder = *positionOrder
				break
			}
		}
	}
	// 如果还有仓位则保留仓位不在开仓
	if holdedOrder.ExchangeID > 0 {
		if s.backtest == false {
			utils.Log.Infof(
				"[WATCHDOG HOLD ORDER - %s] Pair: %s | Price: %v | Quantity: %v  | Side: %s |  OrderFlag: %s",
				holdedOrder.Status,
				guiderPosition.Symbol,
				holdedOrder.Price,
				holdedOrder.Quantity,
				holdedOrder.Side,
				holdedOrder.OrderFlag,
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
		currentPirce,
		amount,
		finalSide,
	)
	// 根据最新价格创建限价单
	_, err = s.broker.CreateOrderLimit(finalSide, model.PositionSideType(guiderPosition.PositionSide), guiderPosition.Symbol, amount, currentPirce, model.OrderExtra{
		Leverage:       config.Leverage,
		GuiderPrice:    guiderPosition.EntryPrice,
		GuiderQuantity: guiderPosition.PositionAmount,
		GuiderAmount:   guiderPosition.PositionInitialMargin,
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
	existOrders, err := s.getExistOrders(s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var tempSideType model.SideType
	//var currentGuiderPosition model.GuiderPosition
	//var processQuantity float64
	for pair, existOrderPositionMap := range existOrders {
		// 同方向订单可能有多个，需要合并订单数量
		for postionSide, orders := range existOrderPositionMap {
			// 获取平仓方向
			if postionSide == model.PositionSideTypeLong {
				tempSideType = model.SideTypeSell
			} else {
				tempSideType = model.SideTypeBuy
			}
			for _, order := range orders {
				// 未查询到该交易对仓位时，平掉该交易对所有仓位
				if _, ok := userPositions[pair]; !ok {
					_, err := s.broker.CreateOrderMarket(tempSideType, postionSide, pair, order.Quantity, model.OrderExtra{
						Leverage:       order.Leverage,
						OrderFlag:      order.OrderFlag,
						GuiderPrice:    order.GuiderPrice,
						GuiderQuantity: order.GuiderQuantity,
						GuiderAmount:   order.GuiderAmount,
					})
					if err != nil {
						utils.Log.Error(err)
						return
					}
				} else {
					// 判断当前订单方向的仓位还在不在，不在平仓
					if _, ok := userPositions[pair][postionSide]; !ok {
						_, err := s.broker.CreateOrderMarket(tempSideType, postionSide, pair, order.Quantity, model.OrderExtra{
							Leverage:       order.Leverage,
							OrderFlag:      order.OrderFlag,
							GuiderPrice:    order.GuiderPrice,
							GuiderQuantity: order.GuiderQuantity,
							GuiderAmount:   order.GuiderAmount,
						})
						if err != nil {
							utils.Log.Error(err)
							return
						}
					}
					continue
					// 加减仓操作
					//currentGuiderPosition = userPositions[pair][postionSide][0]
					//if order.GuiderQuantity == currentGuiderPosition.PositionAmount {
					//	continue
					//}
					//processQuantity = order.Quantity * (order.GuiderQuantity - currentGuiderPosition.PositionAmount) / order.GuiderQuantity
					//if processQuantity > 0 {
					//	// 减仓操作
					//	_, err := s.broker.CreateOrderMarket(tempSideType, postionSide, pair, processQuantity, model.OrderExtra{
					//		Leverage:       order.Leverage,
					//		OrderFlag:      order.OrderFlag,
					//		GuiderPrice:    order.GuiderPrice,
					//		GuiderQuantity: order.GuiderQuantity,
					//		GuiderAmount:   order.GuiderAmount,
					//	})
					//	if err != nil {
					//		utils.Log.Error(err)
					//		return
					//	}
					//} else {
					//	// 加仓操作
					//	_, err := s.broker.CreateOrderMarket(order.Side, postionSide, pair, calc.Abs(processQuantity), model.OrderExtra{
					//		Leverage:       order.Leverage,
					//		OrderFlag:      order.OrderFlag,
					//		GuiderPrice:    order.GuiderPrice,
					//		GuiderQuantity: order.GuiderQuantity,
					//		GuiderAmount:   order.GuiderAmount,
					//	})
					//	if err != nil {
					//		utils.Log.Error(err)
					//		return
					//	}
					//}
				}
			}

			// 更新订单
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
	if longShortRatio > 0.5 {
		finalSide = model.SideTypeBuy
	} else {
		finalSide = model.SideTypeSell
	}
	// 当前仓位为多，最近策略为多，保持仓位
	if assetPosition > 0 && finalSide == model.SideTypeBuy {
		return
	}
	// 当前仓位为空，最近策略为空，保持仓位
	if assetPosition < 0 && finalSide == model.SideTypeSell {
		return
	}
	var tempSideType model.SideType
	var postionSide model.PositionSideType

	// 策略通过，判断当前是否已有未成交的限价单
	// 判断之前是否已有未成交的限价单
	// 直接获取当前交易对订单
	// 原始为空 止损为多  当前为多
	// 判断当前是否已有限价止损单
	// 有限价止损单时，判断止损方向和当前方向一致说明反向了
	// 在判断新的多空比和开仓多空比的大小，新的多空比绝对值比旧的小，需要继续持仓
	// 反之取消所有的限价止损单
	// ----------------
	currentPirce := s.pairPrices[option.Pair]
	holdedOrder := model.Order{}
	existOrderMap, err := s.getPairExistOrders(option, s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for orderFlag, existOrders := range existOrderMap {
		positionOrders, ok := existOrders["position"]
		if !ok {
			continue
		}
		// 获取所有仓位，包含new 和filled
		for _, positionOrder := range positionOrders {
			// 原始单方向和当前策略判断方向相同，保留原始单
			if positionOrder.Side == finalSide {
				holdedOrder = *positionOrder
				continue
			}
			// 计算相对分界线距离，不够反手标准则继续持仓
			if calc.Abs(0.5-longShortRatio) < calc.Abs(0.5-positionOrder.LongShortRatio) {
				holdedOrder = *positionOrder
				continue
			}
			// ******* 开始执行反手交易 ******
			// 取消未成交的挂单
			if positionOrder.Status == model.OrderStatusTypeNew {
				err = s.broker.Cancel(*positionOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
			// 平仓已成交的订单
			if positionOrder.Status == model.OrderStatusTypeFilled {
				if positionOrder.Side == model.SideTypeBuy {
					tempSideType = model.SideTypeSell
				} else {
					tempSideType = model.SideTypeBuy
				}
				// 判断仓位方向为反方向，平掉现有仓位
				_, err := s.broker.CreateOrderMarket(tempSideType, positionOrder.PositionSide, option.Pair, positionOrder.Quantity, model.OrderExtra{
					Leverage:       option.Leverage,
					OrderFlag:      positionOrder.OrderFlag,
					LongShortRatio: positionOrder.LongShortRatio,
					MatchStrategy:  positionOrder.MatchStrategy,
				})
				if err != nil {
					utils.Log.Error(err)
					return
				}
				// 删除止损时间限制配置
				delete(s.lossLimitTimes, positionOrder.OrderFlag)
				utils.Log.Infof(
					"[REVERSE ORDER - %s ] Pair: %s | Price Open: %v, Close: %v | Quantity: %v |  OrderFlag: %s",
					positionOrder.PositionSide,
					option.Pair,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					positionOrder.OrderFlag,
				)
			}
			// 查询当前orderFlag所有的止损单，全部取消
			lossLimitOrders, ok := existOrderMap[orderFlag]["lossLimit"]
			if !ok {
				continue
			}
			// 循环取消
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
	// 如果还有仓位则保留仓位不在开仓
	if holdedOrder.ExchangeID > 0 {
		if s.backtest == false {
			utils.Log.Infof(
				"[HOLD ORDER - %s] Pair: %s | Price: %v | Quantity: %v  | Side: %s |  OrderFlag: %s",
				holdedOrder.Status,
				option.Pair,
				holdedOrder.Price,
				holdedOrder.Quantity,
				holdedOrder.Side,
				holdedOrder.OrderFlag,
			)
		}
		return
	}

	// 根据多空比动态计算仓位大小
	scoreRadio := calc.Abs(0.5-longShortRatio) / 0.5
	amount := calc.OpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPirce, scoreRadio, s.fullSpaceRadio)
	if s.backtest == false {
		utils.Log.Infof(
			"[OPEN] Pair: %s | Price: %v | Quantity: %v | Side: %s",
			option.Pair,
			currentPirce,
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
	order, err := s.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPirce, model.OrderExtra{
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
	existOrderMap, err := s.getPairExistOrders(option, s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var tempSideType model.SideType
	var stopLossDistance float64
	var stopLimitPrice float64

	currentPirce := s.pairPrices[option.Pair]
	loc, err := time.LoadLocation("Asia/Shanghai")
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
			// 当前订单未成交，跳过，等待超时检查process处理
			if positionOrder.Status == model.OrderStatusTypeNew {
				continue
			}
			// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
			if _, ok := s.lossLimitTimes[positionOrder.OrderFlag]; !ok {
				s.lossLimitTimes[positionOrder.OrderFlag] = positionOrder.UpdatedAt.Add(time.Duration(s.lossTimeDuration) * time.Minute)
			}
			profitRatio := calc.ProfitRatio(positionOrder.Side, positionOrder.Price, currentPirce, float64(option.Leverage), positionOrder.Quantity)
			if s.backtest == false {
				utils.Log.Infof(
					"[WATCH] %s Pair: %s | Price Order: %v, Current: %v | Quantity: %v | Profit Ratio: %s | Stop Loss Time: %s",
					positionOrder.UpdatedAt.In(loc).Format("2006-01-02 15:04:05"),
					option.Pair,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					s.lossLimitTimes[positionOrder.OrderFlag].In(loc).Format("2006-01-02 15:04:05"),
				)
			}
			if positionOrder.Side == model.SideTypeBuy {
				tempSideType = model.SideTypeSell
			} else {
				tempSideType = model.SideTypeBuy
			}
			// 如果利润比大于预设值，则使用计算出得利润比 - 指定步进的利润比 得到新的止损利润比
			// 小于预设值，判断止损时间
			// 此处处理时间止损
			if profitRatio < s.initProfitRatioLimit || profitRatio <= (s.profitRatioLimit[option.Pair]+s.profitableScale+0.01) {
				if currentTime.Before(s.lossLimitTimes[positionOrder.OrderFlag]) {
					continue
				}
				// 时间超过限制时间，执行时间止损 市价平单
				_, err := s.broker.CreateOrderMarket(tempSideType, positionOrder.PositionSide, option.Pair, positionOrder.Quantity, model.OrderExtra{
					Leverage:       option.Leverage,
					OrderFlag:      positionOrder.OrderFlag,
					LongShortRatio: positionOrder.LongShortRatio,
					MatchStrategy:  positionOrder.MatchStrategy,
				})
				if err != nil {
					// 如果重新挂限价止损失败则不在取消
					utils.Log.Error(err)
					return
				}
				utils.Log.Infof(
					"[TIMEOUT ORDER - %s ] Pair: %s | Price Open: %v, Close: %v | Quantity: %v |  OrderFlag: %s",
					positionOrder.PositionSide,
					option.Pair,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					positionOrder.OrderFlag,
				)
				// 删除止损时间限制配置
				delete(s.lossLimitTimes, positionOrder.OrderFlag)
				// 盈利利润由开仓时统一重置 不在处理
				// 取消所有的市价止损单
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
				continue
			}
			// 盈利时更新止损终止时间
			s.lossLimitTimes[positionOrder.OrderFlag] = currentTime.Add(time.Duration(s.lossTimeDuration) * time.Minute)
			// 递增利润比
			currentLossLimitProfit := profitRatio - s.profitableScale
			// 使用新的止损利润比计算止损点数
			stopLossDistance = calc.StopLossDistance(currentLossLimitProfit, positionOrder.Price, float64(option.Leverage), positionOrder.Quantity)
			// 重新计算止损价格
			if positionOrder.Side == model.SideTypeSell {
				stopLimitPrice = positionOrder.Price - stopLossDistance
			} else {
				stopLimitPrice = positionOrder.Price + stopLossDistance
			}
			if s.backtest == false {
				utils.Log.Infof(
					"[PROFIT] Pair: %s | Side: %s | Order Price: %v, Current: %v | Quantity: %v | Profit Ratio: %s | New Stop Loss: %v, %s, Time: %s",
					option.Pair,
					positionOrder.Side,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					stopLimitPrice,
					fmt.Sprintf("%.2f%%", currentLossLimitProfit*100),
					s.lossLimitTimes[positionOrder.OrderFlag].In(loc).Format("2006-01-02 15:04:05"),
				)
			}
			// 设置新的止损单
			// 使用滚动利润比保证该止损利润是递增的
			// 不再判断新的止损价格是否小于之前的止损价格
			_, err := s.broker.CreateOrderStopMarket(tempSideType, positionOrder.PositionSide, option.Pair, positionOrder.Quantity, stopLimitPrice, model.OrderExtra{
				Leverage:       option.Leverage,
				OrderFlag:      positionOrder.OrderFlag,
				LongShortRatio: positionOrder.LongShortRatio,
				MatchStrategy:  positionOrder.MatchStrategy,
			})
			if err != nil {
				// 如果重新挂限价止损失败则不在取消
				utils.Log.Error(err)
				continue
			}
			s.profitRatioLimit[option.Pair] = profitRatio - s.profitableScale
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
				continue
			}
		}
	}
}

// map[pair][positionSide]map[status][]Order
func (s *ServiceStrategy) getExistOrders(broker reference.Broker) (map[string]map[model.PositionSideType][]*model.Order, error) {
	// 存储当前存在的仓位和限价单
	existOrders := map[string]map[model.PositionSideType][]*model.Order{}
	positionOrders, err := broker.GetOrdersForOpened()
	if err != nil {
		utils.Log.Error(err)
		return existOrders, err
	}
	if len(positionOrders) > 0 {
		for _, order := range positionOrders {
			if _, ok := existOrders[order.Pair]; !ok {
				existOrders[order.Pair] = make(map[model.PositionSideType][]*model.Order)
			}
			// 同交易对同方向仓位只会有一个,但是可能是由多个订单组成的
			if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeLong) || (order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeShort) {
				// 初始化切片数组
				if _, ok := existOrders[order.Pair][order.PositionSide]; !ok {
					existOrders[order.Pair][order.PositionSide] = []*model.Order{}
				}
				existOrders[order.Pair][order.PositionSide] = append(existOrders[order.Pair][order.PositionSide], order)
			}
		}
	}
	return existOrders, nil
}

func (s *ServiceStrategy) getPairExistOrders(option model.PairOption, broker reference.Broker) (map[string]map[string][]*model.Order, error) {
	// 存储当前存在的仓位和限价单
	existOrders := map[string]map[string][]*model.Order{}
	positionOrders, err := broker.GetOrdersForPairOpened(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return existOrders, err
	}
	if len(positionOrders) > 0 {
		for _, order := range positionOrders {
			if _, ok := existOrders[order.OrderFlag]; !ok {
				existOrders[order.OrderFlag] = make(map[string][]*model.Order)
			}
			// 获取所有开仓单子
			if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeLong) || (order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeShort) {
				if _, ok := existOrders[order.OrderFlag]["position"]; !ok {
					existOrders[order.OrderFlag]["position"] = []*model.Order{}
				}
				existOrders[order.OrderFlag]["position"] = append(existOrders[order.OrderFlag]["position"], order)
			}
			// 获取所有平仓单子
			if (order.Side == model.SideTypeBuy && order.PositionSide == model.PositionSideTypeShort) || (order.Side == model.SideTypeSell && order.PositionSide == model.PositionSideTypeLong) {
				if _, ok := existOrders[order.OrderFlag]["lossLimit"]; !ok {
					existOrders[order.OrderFlag]["lossLimit"] = []*model.Order{}
				}
				existOrders[order.OrderFlag]["lossLimit"] = append(existOrders[order.OrderFlag]["lossLimit"], order)
			}
		}
	}
	return existOrders, nil
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
