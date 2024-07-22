package service

import (
	"context"
	"floolishman/model"
	"floolishman/source"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"sync"
	"time"
)

type ServiceGuider struct {
	mu              sync.Mutex
	ctx             context.Context
	storage         storage.Storage
	guiderConfigs   map[string]map[string]string
	source          source.BaseSourceInterface
	ProxyOption     types.ProxyOption
	pairOptions     map[string]model.PairOption
	guiderPositions map[string][]*model.GuiderPosition
}

var (
	CheckGuiderInterval     time.Duration = 5000
	CheckGuiderSymbolConfig time.Duration = 5000
	CheckGuiderPosition     time.Duration = 500
	SyncGuiderPostions      bool          = false
)

func NewServiceGuider(ctx context.Context, guiderConfigs map[string]map[string]string, storage storage.Storage, proxyOption types.ProxyOption, pairOptions []model.PairOption) *ServiceGuider {
	binanceSource := &source.BinanceSource{}
	binanceSource.InitHttpClient(proxyOption)
	options := map[string]model.PairOption{}
	for _, pairOption := range pairOptions {
		options[pairOption.Pair] = pairOption
	}
	return &ServiceGuider{
		ctx:             ctx,
		guiderConfigs:   guiderConfigs,
		source:          binanceSource,
		storage:         storage,
		pairOptions:     options,
		guiderPositions: make(map[string][]*model.GuiderPosition),
	}
}

func (s *ServiceGuider) Start() {
	go func() {
		checkGuiderTick := time.NewTicker(CheckGuiderInterval * time.Millisecond)
		checkConfigTick := time.NewTicker(CheckGuiderSymbolConfig * time.Millisecond)
		checkPositionTick := time.NewTicker(CheckGuiderPosition * time.Millisecond)
		for {
			select {
			// 5秒查询一次当前跟单情况
			case <-checkGuiderTick.C:
				go s.GuiderListener()
			case <-checkConfigTick.C:
				go s.GuiderSymbolConfigListener()
			case <-checkPositionTick.C:
				if SyncGuiderPostions {
					go s.GuiderPositionListener()
				}
			}
		}
	}()
	utils.Log.Info("Guider started.")
}

func (s *ServiceGuider) GuiderListener() {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	guiderItems := []model.GuiderItem{}
	for account, config := range s.guiderConfigs {
		items, err := s.source.GetUserPortfolioList(config)
		if err != nil {
			utils.Log.Error(err)
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
	utils.Log.Infof("[Watchdog] synced all guiders, total:%v", len(guiderItems))
}
func (s *ServiceGuider) GuiderSymbolConfigListener() {
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
	}
	utils.Log.Infof("[Watchdog] synced all guider symbol config, total:%v", len(symbolConfigs))
}

func (s *ServiceGuider) GuiderPositionListener() {
	portfolioIds, guiderPositions, err := s.FetchPosition()
	if err != nil {
		return
	}
	go func() {
		// 保存每个项目的交易对配置
		err = s.storage.CreateGuiderPositions(portfolioIds, guiderPositions)
		if err != nil {
			utils.Log.Error(err)
		}
	}()
	utils.Log.Infof("[Watchdog] synced all guider positions, total:%v", len(guiderPositions))
}

// FetchPairNewOrder 获取当前交易对委托单 map[pair][PositionSide][]model.GuiderPosition
func (s *ServiceGuider) FetchPosition() ([]string, []*model.GuiderPosition, error) {
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
		}
		for _, userPosition := range userPositions {
			// 计算用户可用金额
			userPosition.AvailQuote = guiderItem.MarginBalance + guiderItem.UnrealizedPnl
			// 缓存用户仓位记录到内存
			s.guiderPositions[guiderItem.CopyPortfolioId] = append(s.guiderPositions[guiderItem.CopyPortfolioId], userPosition)
			// 汇总仓位记录
			tempUserPositions = append(tempUserPositions, userPosition)
		}
	}

	return portfolioIds, tempUserPositions, nil
}

func (s *ServiceGuider) GetGuiderPosition() (map[string]map[model.PositionSideType][]model.GuiderPosition, error) {
	guiderPositionMap := map[string]map[model.PositionSideType][]model.GuiderPosition{}
	_, guiderPositions, err := s.FetchPosition()
	if err != nil {
		return guiderPositionMap, err
	}
	return s.formatGuiderPositions(guiderPositions), nil
}

func (s *ServiceGuider) GetGuiderPositionFromCache() (map[string]map[model.PositionSideType][]model.GuiderPosition, error) {
	guiderPosition := map[string]map[model.PositionSideType][]model.GuiderPosition{}
	guiderItems, err := s.storage.GetGuiderItems()
	if err != nil {
		return guiderPosition, err
	}
	tempUserPositions := make([]*model.GuiderPosition, 0)
	for _, guiderItem := range guiderItems {
		tempUserPositions = append(tempUserPositions, s.guiderPositions[guiderItem.CopyPortfolioId]...)
	}
	return s.formatGuiderPositions(tempUserPositions), nil
}

func (s *ServiceGuider) GetGuiderPositionFromLocal() (map[string]map[model.PositionSideType][]model.GuiderPosition, error) {
	guiderPosition := map[string]map[model.PositionSideType][]model.GuiderPosition{}
	guiderItems, err := s.storage.GetGuiderItems()
	if err != nil {
		return guiderPosition, err
	}
	portfolioIds := []string{}
	for _, guiderItem := range guiderItems {
		portfolioIds = append(portfolioIds, guiderItem.CopyPortfolioId)
	}
	tempUserPositions, err := s.storage.GuiderPositions(portfolioIds)
	if err != nil {
		return guiderPosition, err
	}
	return s.formatGuiderPositions(tempUserPositions), nil
}

func (s *ServiceGuider) GetGuiderPairConfig(portfolioId string, pair string) (*model.GuiderSymbolConfig, error) {
	return s.storage.GetSymbolConfigByPortfolioId(portfolioId, pair)
}

func (s *ServiceGuider) formatGuiderPositions(guiderPositions []*model.GuiderPosition) map[string]map[model.PositionSideType][]model.GuiderPosition {
	finalGuiderPosition := map[string]map[model.PositionSideType][]model.GuiderPosition{}
	for _, position := range guiderPositions {
		// 只允许跟随双向持仓
		if model.PositionSideType(position.PositionSide) == model.PositionSideTypeBoth {
			continue
		}
		if _, ok := finalGuiderPosition[position.Symbol]; !ok {
			finalGuiderPosition[position.Symbol] = make(map[model.PositionSideType][]model.GuiderPosition)
		}
		if _, ok := finalGuiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)]; !ok {
			finalGuiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)] = []model.GuiderPosition{}
		}
		finalGuiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)] = append(finalGuiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)], *position)
	}
	return finalGuiderPosition
}
