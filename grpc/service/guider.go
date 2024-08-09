package service

import (
	"context"
	"floolishman/model"
	"floolishman/pbs/guider"
	"floolishman/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
	"time"
)

type ServiceGuider struct {
	mu                  sync.Mutex
	ctx                 context.Context
	guiderWatcherClient guider.GuiderWatcherClient
}

func NewServiceGuider(ctx context.Context, clientHost string) *ServiceGuider {
	// 配置重试和连接管理
	backoffConfig := backoff.Config{
		BaseDelay:  1 * time.Second,
		Multiplier: 1.6,
		Jitter:     0.2,
		MaxDelay:   120 * time.Second,
	}

	conn, err := grpc.NewClient(clientHost,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoffConfig,
			MinConnectTimeout: 20 * time.Second,
		}),
		grpc.WithBlock(),
	)
	if err != nil {
		utils.Log.Fatalf("did not connect: %v", err)
	}

	client := guider.NewGuiderWatcherClient(conn)
	utils.Log.Infof("[GUIDER] Guider Service Connected")
	return &ServiceGuider{
		ctx:                 ctx,
		guiderWatcherClient: client,
	}
}

func (s *ServiceGuider) GetAllPositions() (map[string]map[model.PositionSideType][]model.GuiderPosition, error) {
	guiderPositionMap := map[string]map[model.PositionSideType][]model.GuiderPosition{}

	resp, err := s.guiderWatcherClient.GetAllPositions(s.ctx, &emptypb.Empty{})
	if err != nil {
		return guiderPositionMap, err
	}
	guiderPositions := resp.GetPositions()
	return s.formatGuiderPositions(guiderPositions), nil
}

func (s *ServiceGuider) GetGuiderPositions(portfolioId string) (map[string]map[model.PositionSideType][]model.GuiderPosition, error) {
	guiderPositionMap := map[string]map[model.PositionSideType][]model.GuiderPosition{}

	req := &guider.GetPositionReq{
		PortfolioId: portfolioId,
	}
	resp, err := s.guiderWatcherClient.GetPositions(s.ctx, req)
	if err != nil {
		return guiderPositionMap, err
	}
	guiderPositions := resp.GetPositions()
	return s.formatGuiderPositions(guiderPositions), nil
}

func (s *ServiceGuider) GetGuiderPairConfig(portfolioId string, pair string) (*model.GuiderSymbolConfig, error) {
	req := &guider.GetSymbolConfigReq{
		PortfolioId: portfolioId,
		Pair:        pair,
	}
	resp, err := s.guiderWatcherClient.GetSymbolConfig(s.ctx, req)
	if err != nil {
		return nil, err
	}
	symbolConfig := resp.GetSymbolConfig()
	return &model.GuiderSymbolConfig{
		Account:          symbolConfig.GetAccount(),
		PortfolioId:      symbolConfig.GetPortfolioId(),
		Symbol:           symbolConfig.GetSymbol(),
		MarginType:       symbolConfig.GetMarginType(),
		Leverage:         int(symbolConfig.GetLeverage()),
		MaxNotionalValue: symbolConfig.GetMaxNotionalValue(),
	}, nil
}

func (s *ServiceGuider) formatGuiderPositions(guiderPositions []*guider.GuiderPosition) map[string]map[model.PositionSideType][]model.GuiderPosition {
	finalGuiderPosition := map[string]map[model.PositionSideType][]model.GuiderPosition{}
	for _, position := range guiderPositions {
		// 只允许跟随双向持仓
		if model.PositionSideType(position.GetPositionSide()) == model.PositionSideTypeBoth {
			continue
		}
		if _, ok := finalGuiderPosition[position.GetSymbol()]; !ok {
			finalGuiderPosition[position.GetSymbol()] = make(map[model.PositionSideType][]model.GuiderPosition)
		}
		if _, ok := finalGuiderPosition[position.GetSymbol()][model.PositionSideType(position.GetPositionSide())]; !ok {
			finalGuiderPosition[position.GetSymbol()][model.PositionSideType(position.GetPositionSide())] = []model.GuiderPosition{}
		}
		finalGuiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)] = append(
			finalGuiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)],
			model.GuiderPosition{
				PortfolioId:            position.GetPortfolioId(),
				Symbol:                 position.GetSymbol(),
				PositionSide:           position.GetPositionSide(),
				PositionAmount:         position.GetPositionAmount(),
				EntryPrice:             position.GetEntryPrice(),
				BreakEvenPrice:         position.GetBreakEvenPrice(),
				MarkPrice:              position.GetMarkPrice(),
				UnrealizedProfit:       position.GetUnrealizedProfit(),
				LiquidationPrice:       position.GetLiquidationPrice(),
				IsolatedMargin:         position.GetIsolatedMargin(),
				NotionalValue:          position.GetNotionalValue(),
				Collateral:             position.GetCollateral(),
				IsolatedWallet:         position.GetIsolatedWallet(),
				CumRealized:            position.GetCumRealized(),
				InitialMargin:          position.GetInitialMargin(),
				MaintMargin:            position.GetMaintMargin(),
				AvailQuote:             position.GetAvailQuote(),
				PositionInitialMargin:  position.GetPositionInitialMargin(),
				OpenOrderInitialMargin: position.GetOpenOrderInitialMargin(),
				Adl:                    int(position.GetAdl()),
				AskNotional:            position.GetAskNotional(),
				BidNotional:            position.GetBidNotional(),
			},
		)
	}
	return finalGuiderPosition
}
