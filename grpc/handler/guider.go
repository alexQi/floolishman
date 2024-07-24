package handler

import (
	"context"
	"floolishman/model"
	"floolishman/pbs/guider"
	"floolishman/source"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
	"time"
)

var (
	wg                      sync.WaitGroup
	CheckGuiderInterval     time.Duration = 5000
	CheckGuiderSymbolConfig time.Duration = 5000
	CheckGuiderPosition     time.Duration = 500
	SyncGuiderPostions      bool          = false
)

type Option func(*HandlerGuider)

// HandlerGuider 定义我们的服务
type HandlerGuider struct {
	mu              sync.Mutex
	storage         storage.Storage
	source          source.BaseSourceInterface
	proxyOption     types.ProxyOption
	pairOptions     map[string]model.PairOption
	guiderConfigs   map[string]map[string]string
	guiderPositions map[string][]*model.GuiderPosition
}

func NewGuiderHandler(
	guiderConfigs map[string]map[string]string,
	pairOptions []model.PairOption,
	proxyOption types.ProxyOption,
	storage storage.Storage,
) *HandlerGuider {
	binanceSource := &source.BinanceSource{}
	binanceSource.InitHttpClient(proxyOption)

	options := map[string]model.PairOption{}
	for _, pairOption := range pairOptions {
		options[pairOption.Pair] = pairOption
	}
	handler := &HandlerGuider{
		storage:         storage,
		source:          binanceSource,
		proxyOption:     proxyOption,
		pairOptions:     options,
		guiderConfigs:   guiderConfigs,
		guiderPositions: make(map[string][]*model.GuiderPosition),
	}
	handler.Start()
	return handler
}

func (s *HandlerGuider) Start() {
	go func() {
		checkGuiderTick := time.NewTicker(CheckGuiderInterval * time.Millisecond)
		checkConfigTick := time.NewTicker(CheckGuiderSymbolConfig * time.Millisecond)
		checkPositionTick := time.NewTicker(CheckGuiderPosition * time.Millisecond)
		for {
			select {
			// 5秒查询一次当前跟单情况
			case <-checkGuiderTick.C:
				go s.ItemListen()
			case <-checkConfigTick.C:
				go s.SymbolConfigListen()
			case <-checkPositionTick.C:
				if SyncGuiderPostions == false {
					continue
				}
				go s.PositionListen()
			}
		}
	}()
	utils.Log.Info("Guider started.")
}

func (s *HandlerGuider) ItemListen() {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	guiderItems := []model.GuiderItem{}
	for account, config := range s.guiderConfigs {
		items, err := s.source.GetUserPortfolioList(config)
		if err != nil {
			utils.Log.Error(err)
			return
		}
		for i := range items {
			items[i].Account = account
		}
		// 保存跟单项目
		guiderItems = append(guiderItems, items...)
	}
	// 保存项目列表
	err := s.storage.CreateGuiderItems(guiderItems)
	if err != nil {
		utils.Log.Error(err)
	}
	// 删除
	utils.Log.Infof("[GUIDER] Item updated | Total: %v", len(guiderItems))
}
func (s *HandlerGuider) SymbolConfigListen() {
	guiderItems, err := s.storage.GetGuiderItems()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	symbolConfigs := []model.GuiderSymbolConfig{}
	for _, guiderItem := range guiderItems {
		itemConfigs, err := s.source.CheckGuiderSymbolConfig(guiderItem.CopyPortfolioId, s.guiderConfigs[guiderItem.Account])
		if err != nil {
			utils.Log.Error(err)
			return
		}

		for i := range itemConfigs {
			if _, ok := s.pairOptions[itemConfigs[i].Symbol]; !ok {
				continue
			}
			itemConfigs[i].Account = guiderItem.Account
			itemConfigs[i].PortfolioId = guiderItem.CopyPortfolioId
			symbolConfigs = append(symbolConfigs, itemConfigs[i])
		}
	}
	// 保存每个项目的交易对配置
	err = s.storage.CreateSymbolConfigs(symbolConfigs)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	utils.Log.Infof("[GUIDER] PairConfig updated | Total: %v", len(symbolConfigs))
}

func (s *HandlerGuider) PositionListen() {
	portfolioIds, guiderPositions, err := s.FetchPosition()
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 保存每个项目的交易对配置
	err = s.storage.CreateGuiderPositions(portfolioIds, guiderPositions)
	if err != nil {
		utils.Log.Error(err)
	}
	utils.Log.Infof("[GUIDER] Position updated | T otal: %v", len(guiderPositions))
}

// FetchPairNewOrder 获取当前交易对委托单 map[pair][PositionSide][]model.GuiderPosition
func (s *HandlerGuider) FetchPosition() ([]string, []*model.GuiderPosition, error) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	portfolioIds := []string{}
	tempUserPositions := []*model.GuiderPosition{}
	guiderItems, err := s.storage.GetGuiderItems()
	if err != nil {
		// 未获取到跟单员信息，重置跟单员仓位为空
		s.guiderPositions = make(map[string][]*model.GuiderPosition, 0)
		return portfolioIds, tempUserPositions, err
	}
	// 清除guider postion中guider不存在的部分
	guiderItemMap := map[string]model.GuiderItem{}
	for _, guiderItem := range guiderItems {
		guiderItemMap[guiderItem.CopyPortfolioId] = *guiderItem
	}
	for portfolioId := range s.guiderPositions {
		if _, ok := guiderItemMap[portfolioId]; !ok {
			delete(s.guiderPositions, portfolioId)
		}
	}
	for _, guiderItem := range guiderItems {
		// 记录跟单员id
		portfolioIds = append(portfolioIds, guiderItem.CopyPortfolioId)
		// 重新初始化cache
		s.guiderPositions[guiderItem.CopyPortfolioId] = make([]*model.GuiderPosition, 0)
		// 请求接口数据
		userPositions, err := s.source.CheckUserPosition(guiderItem.CopyPortfolioId, s.guiderConfigs[guiderItem.Account])
		if err != nil {
			utils.Log.Error(err)
			return portfolioIds, tempUserPositions, err
		}
		for _, userPosition := range userPositions {
			// 计算用户可用金额
			userPosition.AvailQuote = guiderItem.MarginBalance + guiderItem.UnrealizedPnl
			// 放入CopyPortfolioId
			userPosition.PortfolioId = guiderItem.CopyPortfolioId
			// 缓存用户仓位记录到内存
			s.guiderPositions[guiderItem.CopyPortfolioId] = append(s.guiderPositions[guiderItem.CopyPortfolioId], userPosition)
			// 汇总仓位记录
			tempUserPositions = append(tempUserPositions, userPosition)
		}
	}

	return portfolioIds, tempUserPositions, nil
}

func (s *HandlerGuider) FetchPortfolioPosition(guiderItem *model.GuiderItem) ([]*model.GuiderPosition, error) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	tempUserPositions := []*model.GuiderPosition{}
	// 重新初始化cache
	s.guiderPositions[guiderItem.CopyPortfolioId] = make([]*model.GuiderPosition, 0)
	// 请求接口数据
	userPositions, err := s.source.CheckUserPosition(guiderItem.CopyPortfolioId, s.guiderConfigs[guiderItem.Account])
	if err != nil {
		utils.Log.Error(err)
		return nil, err
	}
	for _, userPosition := range userPositions {
		// 计算用户可用金额
		userPosition.AvailQuote = guiderItem.MarginBalance + guiderItem.UnrealizedPnl
		// 缓存用户仓位记录到内存
		s.guiderPositions[guiderItem.CopyPortfolioId] = append(s.guiderPositions[guiderItem.CopyPortfolioId], userPosition)
		// 汇总仓位记录
		tempUserPositions = append(tempUserPositions, userPosition)
	}

	return tempUserPositions, nil
}

func (c *HandlerGuider) GetItems(ctx context.Context, req *guider.GetItemsReq) (*guider.GetItemResp, error) {
	response := &guider.GetItemResp{}
	items, err := c.storage.GetGuiderItemsByFilter(storage.ItemFilterParams{Account: req.GetAccount()})
	if err != nil {
		return response, err
	}
	for _, item := range items {
		response.GuiderItems = append(response.GuiderItems, &guider.GuiderItem{
			Account:                item.Account,
			AvatarUrl:              item.AvatarUrl,
			Nickname:               item.Nickname,
			CopyPortfolioId:        item.CopyPortfolioId,
			LeadPortfolioId:        item.LeadPortfolioId,
			StartDate:              item.StartDate,
			EndDate:                item.EndDate,
			ClosedReason:           item.ClosedReason,
			NetCopyAmount:          item.NetCopyAmount,
			NetCopyAsset:           item.NetCopyAsset,
			UnrealizedPnl:          item.UnrealizedPnl,
			UnrealizedPnlAsset:     item.UnrealizedPnlAsset,
			RealizedPnl:            item.RealizedPnl,
			RealizedPnlAsset:       item.RealizedPnlAsset,
			NetProfitAmount:        item.NetProfitAmount,
			NetProfitAsset:         item.NetProfitAsset,
			ProfitSharedAmount:     item.ProfitSharedAmount,
			ProfitSharedAsset:      item.ProfitSharedAsset,
			UnProfitSharedAmount:   item.UnProfitSharedAmount,
			MarginBalance:          item.MarginBalance,
			MarginBalanceAsset:     item.MarginBalanceAsset,
			ProfitSharingRate:      item.ProfitSharingRate,
			CopierUnlockPeriodDays: item.CopierUnlockPeriodDays,
			TotalSlRate:            item.TotalSlRate,
		})
	}
	return response, nil
}

func (c *HandlerGuider) GetSymbolConfig(ctx context.Context, req *guider.GetSymbolConfigReq) (*guider.GetSymbolConfigResp, error) {
	response := &guider.GetSymbolConfigResp{}
	symbolConfig, err := c.storage.GetSymbolConfigByPortfolioId(req.GetPortfolioId(), req.GetPair())
	if err != nil {
		return response, err
	}
	response.SymbolConfig = &guider.GuiderSymbolConfig{
		Account:          symbolConfig.Account,
		Symbol:           symbolConfig.Symbol,
		PortfolioId:      symbolConfig.PortfolioId,
		MarginType:       symbolConfig.MarginType,
		Leverage:         int32(symbolConfig.Leverage),
		MaxNotionalValue: symbolConfig.MaxNotionalValue,
	}
	return response, nil
}

func (c *HandlerGuider) GetPositions(ctx context.Context, req *guider.GetPositionReq) (*guider.GetPositionResp, error) {
	response := &guider.GetPositionResp{}
	guiderItem, err := c.storage.GetGuiderItemByPortfolioId(req.GetPortfolioId())
	if err != nil {
		return response, err
	}
	positions, err := c.FetchPortfolioPosition(guiderItem)
	if err != nil {
		return response, err
	}
	for _, position := range positions {
		response.Positions = append(response.Positions, &guider.GuiderPosition{
			PortfolioId:            req.PortfolioId,
			Symbol:                 position.Symbol,
			PositionSide:           position.PositionSide,
			PositionAmount:         position.PositionAmount,
			EntryPrice:             position.EntryPrice,
			BreakEvenPrice:         position.BreakEvenPrice,
			MarkPrice:              position.MarkPrice,
			UnrealizedProfit:       position.UnrealizedProfit,
			LiquidationPrice:       position.LiquidationPrice,
			IsolatedMargin:         position.IsolatedMargin,
			NotionalValue:          position.NotionalValue,
			Collateral:             position.Collateral,
			IsolatedWallet:         position.IsolatedWallet,
			CumRealized:            position.CumRealized,
			InitialMargin:          position.InitialMargin,
			MaintMargin:            position.MaintMargin,
			AvailQuote:             position.AvailQuote,
			PositionInitialMargin:  position.PositionInitialMargin,
			OpenOrderInitialMargin: position.OpenOrderInitialMargin,
			Adl:                    int32(position.Adl),
			AskNotional:            position.AskNotional,
			BidNotional:            position.BidNotional,
		})
	}
	return response, nil
}

func (c *HandlerGuider) GetAllPositions(context.Context, *emptypb.Empty) (*guider.GetPositionResp, error) {
	response := &guider.GetPositionResp{}
	_, positions, err := c.FetchPosition()
	if err != nil {
		return response, err
	}
	for _, position := range positions {
		response.Positions = append(response.Positions, &guider.GuiderPosition{
			PortfolioId:            position.PortfolioId,
			Symbol:                 position.Symbol,
			PositionSide:           position.PositionSide,
			PositionAmount:         position.PositionAmount,
			EntryPrice:             position.EntryPrice,
			BreakEvenPrice:         position.BreakEvenPrice,
			MarkPrice:              position.MarkPrice,
			UnrealizedProfit:       position.UnrealizedProfit,
			LiquidationPrice:       position.LiquidationPrice,
			IsolatedMargin:         position.IsolatedMargin,
			NotionalValue:          position.NotionalValue,
			Collateral:             position.Collateral,
			IsolatedWallet:         position.IsolatedWallet,
			CumRealized:            position.CumRealized,
			InitialMargin:          position.InitialMargin,
			MaintMargin:            position.MaintMargin,
			AvailQuote:             position.AvailQuote,
			PositionInitialMargin:  position.PositionInitialMargin,
			OpenOrderInitialMargin: position.OpenOrderInitialMargin,
			Adl:                    int32(position.Adl),
			AskNotional:            position.AskNotional,
			BidNotional:            position.BidNotional,
		})
	}
	return response, nil
}
