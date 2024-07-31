package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"github.com/adshao/go-binance/v2/futures"
	"time"
)

type CallerWatchdog struct {
	CallerCommon
}

func (c *CallerWatchdog) Start() {
	go func() {
		tickerCheck := time.NewTicker(CheckStrategyInterval * time.Second)
		tickerClose := time.NewTicker(CheckCloseInterval * time.Millisecond)
		tickerLeverage := time.NewTicker(CheckLeverageInterval * time.Millisecond)
		for {
			select {
			case <-tickerCheck.C:
				c.checkPosition()
			case <-tickerClose.C:
				if c.setting.FollowSymbol {
					c.checkPositionClose()
				}
			case <-tickerLeverage.C:
				if c.setting.FollowSymbol {
					c.checkPositionLeverage()
				}
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}()
	c.Listen()
}

func (cc *CallerWatchdog) Listen() {
	// 执行超时检查
	go cc.tickCheckOrderTimeout()
	// 非回溯测试模式且不是看门狗方式下监听平仓
	if cc.setting.FollowSymbol == false {
		go cc.tickerCheckForClose(cc.pairOptions)
	}
}

func (c *CallerWatchdog) checkPosition() {
	userPositions, err := c.guider.GetAllPositions()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(userPositions) == 0 {
		return
	}
	// 跟随模式下，开仓平仓都跟随看门狗，多跟模式下开仓保持不变什么
	// todo 对冲仓位时如何处理
	var currentUserPosition model.GuiderPosition
	if c.setting.FollowSymbol {
		for _, userPosition := range userPositions {
			if len(userPosition) > 1 {
				continue
			}
			if _, ok := userPosition[model.PositionSideTypeLong]; !ok {
				currentUserPosition = userPosition[model.PositionSideTypeShort][0]
			}
			if _, ok := userPosition[model.PositionSideTypeShort]; !ok {
				currentUserPosition = userPosition[model.PositionSideTypeLong][0]
			}
			// 屏蔽未配置的交易对
			if _, ok := c.pairOptions[currentUserPosition.Symbol]; !ok {
				continue
			}
			go c.openWatchdogPosition(currentUserPosition)
		}
	} else {
		var longShortRatio float64
		for _, option := range c.pairOptions {
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
			assetPosition, quotePosition, err := c.broker.PairAsset(option.Pair)
			if err != nil {
				utils.Log.Error(err)
			}
			c.openPosition(
				option,
				assetPosition,
				quotePosition,
				longShortRatio,
				map[string]int{"watchdog": 1},
				[]model.Strategy{},
			)
		}
	}
}

func (s *CallerWatchdog) openWatchdogPosition(guiderPosition model.GuiderPosition) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	currentPrice := s.pairPrices[guiderPosition.Symbol]
	// 判断当前资产
	assetPosition, quotePosition, err := s.broker.PairAsset(guiderPosition.Symbol)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	// 当前仓位为多，最近策略为多，保持仓位
	// 当前仓位为空，最近策略为空，保持仓位
	if (assetPosition > 0 && model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeLong) ||
		(assetPosition < 0 && model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeShort) {
		utils.Log.Infof(
			"[POSITION EXSIT]  Pair: %s | P.Side: %s | Quantity: %v",
			guiderPosition.Symbol,
			guiderPosition.PositionSide,
			assetPosition,
		)
		return
	}
	// 获取当前交易对配置
	config, err := s.guider.GetGuiderPairConfig(guiderPosition.PortfolioId, guiderPosition.Symbol)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 对方的保证金占比
	amount := ((guiderPosition.InitialMargin / guiderPosition.AvailQuote) * quotePosition * float64(config.Leverage)) / currentPrice

	var finalSide model.SideType
	isGuiderProfited := false
	if model.PositionSideType(guiderPosition.PositionSide) == model.PositionSideTypeLong {
		finalSide = model.SideTypeBuy
		if currentPrice > guiderPosition.EntryPrice {
			isGuiderProfited = true
		}
	} else {
		finalSide = model.SideTypeSell
		if currentPrice < guiderPosition.EntryPrice {
			isGuiderProfited = true
		}
	}
	// 当guider盈利时不在开仓，点位保持比guier优
	if isGuiderProfited == true {
		utils.Log.Infof(
			"[GUIDER POSITION - IGNORE] Pair: %s | P.Side: %s | Price: %v, Current: %v",
			guiderPosition.Symbol,
			guiderPosition.PositionSide,
			guiderPosition.EntryPrice,
			currentPrice,
		)
		return
		//profitRatio := calc.ProfitRatio(finalSide, guiderPosition.EntryPrice, currentPrice, float64(config.Leverage), amount)
		//if profitRatio > 0.12/100*float64(config.Leverage) {
		//	utils.Log.Infof(
		//		"[IGNORE ORDER] Pair: %s, Price: %v, Quantity: %v, P.Side: %s | Current: %v | (%.f)",
		//		guiderPosition.Symbol,
		//		guiderPosition.EntryPrice,
		//		guiderPosition.PositionAmount,
		//		guiderPosition.PositionSide,
		//		currentPrice,
		//		profitRatio,
		//	)
		//	return
		//}
	}
	utils.Log.Infof(
		"[GUIDER POSITION] Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v ｜ PortfolioId: %s",
		guiderPosition.Symbol,
		guiderPosition.PositionSide,
		guiderPosition.PositionAmount,
		guiderPosition.EntryPrice,
		currentPrice,
		guiderPosition.PortfolioId,
	)
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
	if existPosition != nil {
		utils.Log.Infof(
			"[WATCHDOG POSITION - EXSIT] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v",
			existPosition.OrderFlag,
			existPosition.Pair,
			existPosition.PositionSide,
			existPosition.Quantity,
			existPosition.AvgPrice,
			currentPrice,
		)
		return
	}
	hasOrder, err := s.CheckHasUnfilledPositionOrders(guiderPosition.Symbol, finalSide, model.PositionSideType(guiderPosition.PositionSide))
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if hasOrder {
		return
	}
	// 设置当前交易对信息
	err = s.exchange.SetPairOption(s.ctx, model.PairOption{
		Pair:       config.Symbol,
		Leverage:   config.Leverage,
		MarginType: futures.MarginType(config.MarginType),
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	utils.Log.Infof(
		"[WATCHDOG POSITION OPENING] Pair: %s | P.Side: %s | Quantity: %v | Price: %v | GuiderOrigin: %s",
		guiderPosition.Symbol,
		guiderPosition.PositionSide,
		amount,
		currentPrice,
		guiderPosition.PortfolioId,
	)
	// 根据最新价格创建限价单
	_, err = s.broker.CreateOrderLimit(finalSide, model.PositionSideType(guiderPosition.PositionSide), guiderPosition.Symbol, amount, currentPrice, model.OrderExtra{
		Leverage:       config.Leverage,
		PositionAmount: calc.Abs(guiderPosition.PositionAmount),
		GuiderOrigin:   guiderPosition.PortfolioId, // 仓位识别表示，判定交易员
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
}

func (s *CallerWatchdog) checkPositionLeverage() {
	openedPositions, err := s.broker.GetPositionsForOpened()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 循环当前仓位，查询当前仓位杠杆信息有没有变动
	// 本地仓位与guider仓位杠杆倍数不一致时重新设置杠杆倍数
	// order服务中监听本地仓位信息，更新杠杆倍数
	for _, openedPosition := range openedPositions {
		// 获取当前交易对配置
		config, err := s.guider.GetGuiderPairConfig(openedPosition.GuiderOrigin, openedPosition.Pair)
		if err != nil {
			utils.Log.Error(err)
			return
		}
		if config.Leverage == openedPosition.Leverage {
			continue
		}
		// 设置当前交易对信息
		err = s.exchange.SetPairOption(s.ctx, model.PairOption{
			Pair:       config.Symbol,
			Leverage:   config.Leverage,
			MarginType: futures.MarginType(config.MarginType),
		})
		if err != nil {
			utils.Log.Error(err)
			return
		}
		utils.Log.Infof(
			"[WATCHDOG LEVERAGE] OrderFlag: %s | Pair: %s | Origin: %v, Current: %v",
			openedPosition.OrderFlag,
			openedPosition.Pair,
			openedPosition.Leverage,
			config.Leverage,
		)
	}
}

func (s *CallerWatchdog) checkPositionClose() {
	// 查询用户仓位
	userPositions, err := s.guider.GetAllPositions()
	if err != nil {
		//todo 需要统计错误次数 ，错误次数太多需要发送通知
		utils.Log.Error(err)
		return
	}
	openedPositions, err := s.broker.GetPositionsForOpened()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 平仓时判断当前仓位GuiderOrigin 是否还在
	var hasPendingOrder bool
	var closeSideType model.SideType
	var guiderPositionAmount, processQuantity, currentQuantity, currentPrice float64
	// 循环当前仓位，根据仓位orderFlag查询当前仓位关联的所有订单
	for _, openedPosition := range openedPositions {
		// 获取平仓方向
		if openedPosition.PositionSide == string(model.PositionSideTypeLong) {
			closeSideType = model.SideTypeSell
		} else {
			closeSideType = model.SideTypeBuy
		}
		currentPrice = s.pairPrices[openedPosition.Pair]
		// 判断当前币种仓位是否在guider中存在，不存在时平掉全部仓位 （所有guider都没有该方向仓位则该订单已被平仓）
		if _, ok := userPositions[openedPosition.Pair]; !ok {
			_, err := s.broker.CreateOrderMarket(
				closeSideType,
				model.PositionSideType(openedPosition.PositionSide),
				openedPosition.Pair,
				openedPosition.Quantity,
				model.OrderExtra{
					OrderFlag:    openedPosition.OrderFlag,
					Leverage:     openedPosition.Leverage,
					GuiderOrigin: openedPosition.GuiderOrigin,
				},
			)
			if err != nil {
				utils.Log.Error(err)
				return
			}
		} else {
			// 判断用户当前方向仓位是否在guider的仓位中，不在则继续平仓 （所有guider都没有该方向仓位）
			currentGuiderPositions, ok := userPositions[openedPosition.Pair][model.PositionSideType(openedPosition.PositionSide)]
			if !ok {
				_, err := s.broker.CreateOrderMarket(
					closeSideType,
					model.PositionSideType(openedPosition.PositionSide),
					openedPosition.Pair,
					openedPosition.Quantity,
					model.OrderExtra{
						OrderFlag:    openedPosition.OrderFlag,
						Leverage:     openedPosition.Leverage,
						GuiderOrigin: openedPosition.GuiderOrigin,
					},
				)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			} else {
				// 判断当前guiderPosition是否与当前仓位同源
				if openedPosition.GuiderOrigin != currentGuiderPositions[0].PortfolioId {
					continue
				}
				// 同源仓位存在，判断当前仓位比例是否和之前一致,一致时跳过
				guiderPositionAmount = calc.Abs(currentGuiderPositions[0].PositionAmount)
				if calc.FloatEquals(openedPosition.Quantity/guiderPositionAmount, openedPosition.GuiderPositionRate, 0.02) {
					utils.Log.Infof(
						"[WATCHDOG POSITION - WATCH] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v",
						openedPosition.OrderFlag,
						openedPosition.Pair,
						openedPosition.PositionSide,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						currentPrice,
					)
					continue
				}
				currentQuantity = calc.RoundToDecimalPlaces(guiderPositionAmount*openedPosition.GuiderPositionRate, 3)
				// 获取当前要加减仓的数量
				processQuantity = calc.AccurateSub(openedPosition.Quantity, currentQuantity)
				// 判断当前是否已有加仓减仓的单子
				existUnfilledOrderMap, err := s.broker.GetOrdersForPairUnfilled(openedPosition.Pair)
				if err != nil {
					utils.Log.Error(err)
					return
				}
				// 仓位比之前小，减仓
				if processQuantity > 0 {
					// 判断是否已有减仓订单
					if _, ok := existUnfilledOrderMap[openedPosition.OrderFlag]; ok {
						positionOrders, ok := existUnfilledOrderMap[openedPosition.OrderFlag]["lossLimit"]
						if ok {
							// 判断当前是否有同向挂单
							for _, positionOrder := range positionOrders {
								if positionOrder.Side != model.SideType(openedPosition.Side) && positionOrder.PositionSide == model.PositionSideType(openedPosition.PositionSide) {
									hasPendingOrder = true
									break
								}
							}
						}
					}
					if hasPendingOrder {
						return
					}
					// 处理精度波动导致无法完全平仓
					if processQuantity/openedPosition.Quantity > 0.97 {
						processQuantity = openedPosition.Quantity
					}
					// 减仓或者平仓
					_, err := s.broker.CreateOrderMarket(
						closeSideType,
						model.PositionSideType(openedPosition.PositionSide),
						openedPosition.Pair,
						processQuantity,
						model.OrderExtra{
							Leverage:       openedPosition.Leverage,
							OrderFlag:      openedPosition.OrderFlag,
							PositionAmount: guiderPositionAmount,
							GuiderOrigin:   openedPosition.GuiderOrigin,
						},
					)
					if err != nil {
						utils.Log.Error(err)
						return
					}
				} else {
					// 判断是否已有加仓订单
					if _, ok := existUnfilledOrderMap[openedPosition.OrderFlag]; ok {
						positionOrders, ok := existUnfilledOrderMap[openedPosition.OrderFlag]["position"]
						if ok {
							// 判断当前是否有同向挂单
							for _, positionOrder := range positionOrders {
								if positionOrder.Side == model.SideType(openedPosition.Side) && positionOrder.PositionSide == model.PositionSideType(openedPosition.PositionSide) {
									hasPendingOrder = true
									break
								}
							}
						}
					}
					if hasPendingOrder {
						return
					}
					// 加仓操作
					_, err := s.broker.CreateOrderMarket(
						model.SideType(openedPosition.Side),
						model.PositionSideType(openedPosition.PositionSide),
						openedPosition.Pair,
						calc.Abs(processQuantity),
						model.OrderExtra{
							Leverage:       openedPosition.Leverage,
							OrderFlag:      openedPosition.OrderFlag,
							PositionAmount: guiderPositionAmount,
							GuiderOrigin:   openedPosition.GuiderOrigin,
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
