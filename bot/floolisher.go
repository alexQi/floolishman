package bot

import (
	"bytes"
	"context"
	"floolisher/types"
	"fmt"
	"os"
	"strconv"

	"github.com/aybabtme/uniplot/histogram"

	"floolisher/exchange"
	"floolisher/handler"
	"floolisher/model"
	"floolisher/notification"
	"floolisher/order"
	"floolisher/service"
	"floolisher/storage"
	_ "floolisher/tools/config"
	"floolisher/tools/log"
	"floolisher/tools/metrics"

	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
)

const defaultDatabase = "floolisher.db"

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04",
	})
}

type OrderSubscriber interface {
	OnOrder(model.Order)
}

type CandleSubscriber interface {
	OnCandle(string, model.Candle)
}

type Floolisher struct {
	storage  storage.Storage
	settings model.Settings
	exchange service.Exchange
	strategy types.CompositesStrategy
	notifier service.Notifier
	telegram service.Telegram

	orderController       *order.Controller
	priorityQueueCandles  map[string]*model.PriorityQueue
	strategiesControllers map[string]*handler.Controller
	orderFeed             *order.Feed
	dataFeed              *exchange.DataFeedSubscription
	paperWallet           *exchange.PaperWallet

	backtest bool
}

type Option func(*Floolisher)

func NewBot(ctx context.Context, settings model.Settings, exch service.Exchange, str types.CompositesStrategy,
	options ...Option) (*Floolisher, error) {

	priorityQueueCandles := map[string]*model.PriorityQueue{}
	for _, s := range str.Strategies {
		priorityQueueCandles[s.Timeframe()] = model.NewPriorityQueue(nil)
	}

	bot := &Floolisher{
		settings:              settings,
		exchange:              exch,
		strategy:              str,
		orderFeed:             order.NewOrderFeed(),
		dataFeed:              exchange.NewDataFeed(exch),
		strategiesControllers: make(map[string]*handler.Controller),
		priorityQueueCandles:  priorityQueueCandles,
	}

	for _, pair := range settings.Pairs {
		asset, quote := exchange.SplitAssetQuote(pair)
		if asset == "" || quote == "" {
			return nil, fmt.Errorf("invalid pair: %s", pair)
		}
	}

	for _, option := range options {
		option(bot)
	}

	var err error
	if bot.storage == nil {
		bot.storage, err = storage.FromFile(defaultDatabase)
		if err != nil {
			return nil, err
		}
	}

	bot.orderController = order.NewController(ctx, exch, bot.storage, bot.orderFeed)

	if settings.Telegram.Enabled {
		bot.telegram, err = notification.NewTelegram(bot.orderController, settings)
		if err != nil {
			return nil, err
		}
		// register telegram as notifier
		WithNotifier(bot.telegram)(bot)
	}

	return bot, nil
}

// WithBacktest sets the bot to run in backtest mode, it is required for backtesting environments
// Backtest mode optimize the input read for CSV and deal with race conditions
func WithBacktest(wallet *exchange.PaperWallet) Option {
	return func(bot *Floolisher) {
		bot.backtest = true
		opt := WithPaperWallet(wallet)
		opt(bot)
	}
}

// WithStorage sets the storage for the bot, by default it uses a local file called ninjabot.db
func WithStorage(storage storage.Storage) Option {
	return func(bot *Floolisher) {
		bot.storage = storage
	}
}

// WithLogLevel sets the log level. eg: log.DebugLevel, log.InfoLevel, log.WarnLevel, log.ErrorLevel, log.FatalLevel
func WithLogLevel(level log.Level) Option {
	return func(_ *Floolisher) {
		log.SetLevel(level)
	}
}

// WithNotifier registers a notifier to the bot, currently only email and telegram are supported
func WithNotifier(notifier service.Notifier) Option {
	return func(bot *Floolisher) {
		bot.notifier = notifier
		bot.orderController.SetNotifier(notifier)
		bot.SubscribeOrder(notifier)
	}
}

// WithCandleSubscription subscribes a given struct to the candle feed
func WithCandleSubscription(subscriber CandleSubscriber) Option {
	return func(bot *Floolisher) {
		bot.SubscribeCandle(subscriber)
	}
}

// WithPaperWallet sets the paper wallet for the bot (used for backtesting and live simulation)
func WithPaperWallet(wallet *exchange.PaperWallet) Option {
	return func(bot *Floolisher) {
		bot.paperWallet = wallet
	}
}

func (n *Floolisher) SubscribeCandle(subscriptions ...CandleSubscriber) {
	for _, pair := range n.settings.Pairs {
		for _, subscription := range subscriptions {
			for _, s := range n.strategy.Strategies {
				n.dataFeed.Subscribe(pair, s.Timeframe(), subscription.OnCandle, false)
			}
		}
	}
}

func WithOrderSubscription(subscriber OrderSubscriber) Option {
	return func(bot *Floolisher) {
		bot.SubscribeOrder(subscriber)
	}
}

func (n *Floolisher) SubscribeOrder(subscriptions ...OrderSubscriber) {
	for _, pair := range n.settings.Pairs {
		for _, subscription := range subscriptions {
			n.orderFeed.Subscribe(pair, subscription.OnOrder, false)
		}
	}
}

func (n *Floolisher) Controller() *order.Controller {
	return n.orderController
}

// Summary function displays all trades, accuracy and some bot metrics in stdout
// To access the raw data, you may access `bot.Controller().Results`
func (n *Floolisher) Summary() {
	var (
		total  float64
		wins   int
		loses  int
		volume float64
		sqn    float64
	)

	buffer := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buffer)
	table.SetHeader([]string{"Pair", "Trades", "Win", "Loss", "% Win", "Payoff", "Pr Fact.", "SQN", "Profit", "Volume"})
	table.SetFooterAlignment(tablewriter.ALIGN_RIGHT)
	avgPayoff := 0.0
	avgProfitFactor := 0.0

	returns := make([]float64, 0)
	for _, summary := range n.orderController.Results {
		avgPayoff += summary.Payoff() * float64(len(summary.Win())+len(summary.Lose()))
		avgProfitFactor += summary.ProfitFactor() * float64(len(summary.Win())+len(summary.Lose()))
		table.Append([]string{
			summary.Pair,
			strconv.Itoa(len(summary.Win()) + len(summary.Lose())),
			strconv.Itoa(len(summary.Win())),
			strconv.Itoa(len(summary.Lose())),
			fmt.Sprintf("%.1f %%", float64(len(summary.Win()))/float64(len(summary.Win())+len(summary.Lose()))*100),
			fmt.Sprintf("%.3f", summary.Payoff()),
			fmt.Sprintf("%.3f", summary.ProfitFactor()),
			fmt.Sprintf("%.1f", summary.SQN()),
			fmt.Sprintf("%.2f", summary.Profit()),
			fmt.Sprintf("%.2f", summary.Volume),
		})
		total += summary.Profit()
		sqn += summary.SQN()
		wins += len(summary.Win())
		loses += len(summary.Lose())
		volume += summary.Volume

		returns = append(returns, summary.WinPercent()...)
		returns = append(returns, summary.LosePercent()...)
	}

	table.SetFooter([]string{
		"TOTAL",
		strconv.Itoa(wins + loses),
		strconv.Itoa(wins),
		strconv.Itoa(loses),
		fmt.Sprintf("%.1f %%", float64(wins)/float64(wins+loses)*100),
		fmt.Sprintf("%.3f", avgPayoff/float64(wins+loses)),
		fmt.Sprintf("%.3f", avgProfitFactor/float64(wins+loses)),
		fmt.Sprintf("%.1f", sqn/float64(len(n.orderController.Results))),
		fmt.Sprintf("%.2f", total),
		fmt.Sprintf("%.2f", volume),
	})
	table.Render()

	fmt.Println(buffer.String())
	fmt.Println("------ RETURN -------")
	totalReturn := 0.0
	returnsPercent := make([]float64, len(returns))
	for _, p := range returns {
		returnsPercent = append(returnsPercent, p*100)
		totalReturn += p
	}
	hist := histogram.Hist(15, returnsPercent)
	err := histogram.Fprint(os.Stdout, hist, histogram.Linear(10))
	if err != nil {
		return
	}
	fmt.Println()

	fmt.Println("------ CONFIDENCE INTERVAL (95%) -------")
	for pair, summary := range n.orderController.Results {
		fmt.Printf("| %s |\n", pair)
		returns := append(summary.WinPercent(), summary.LosePercent()...)
		returnsInterval := metrics.Bootstrap(returns, metrics.Mean, 10000, 0.95)
		payoffInterval := metrics.Bootstrap(returns, metrics.Payoff, 10000, 0.95)
		profitFactorInterval := metrics.Bootstrap(returns, metrics.ProfitFactor, 10000, 0.95)

		fmt.Printf("RETURN:      %.2f%% (%.2f%% ~ %.2f%%)\n",
			returnsInterval.Mean*100, returnsInterval.Lower*100, returnsInterval.Upper*100)
		fmt.Printf("PAYOFF:      %.2f (%.2f ~ %.2f)\n",
			payoffInterval.Mean, payoffInterval.Lower, payoffInterval.Upper)
		fmt.Printf("PROF.FACTOR: %.2f (%.2f ~ %.2f)\n",
			profitFactorInterval.Mean, profitFactorInterval.Lower, profitFactorInterval.Upper)
	}

	fmt.Println()

	if n.paperWallet != nil {
		n.paperWallet.Summary()
	}

}

func (n Floolisher) SaveReturns(outputDir string) error {
	for _, summary := range n.orderController.Results {
		outputFile := fmt.Sprintf("%s/%s.csv", outputDir, summary.Pair)
		if err := summary.SaveReturns(outputFile); err != nil {
			return err
		}
	}
	return nil
}

func (n *Floolisher) onCandle(timeframe string, candle model.Candle) {
	n.priorityQueueCandles[timeframe].Push(candle)
}

func (n *Floolisher) processCandle(timeframe string, candle model.Candle) {
	if n.paperWallet != nil {
		n.paperWallet.OnCandle(candle)
	}

	n.strategiesControllers[candle.Pair].OnPartialCandle(candle)
	if candle.Complete {
		n.strategiesControllers[candle.Pair].OnCandle(timeframe, candle)
		n.orderController.OnCandle(candle)
	}
}

// Process pending candles in buffer
func (n *Floolisher) processCandles() {
	for _, s := range n.strategy.Strategies {
		for item := range n.priorityQueueCandles[s.Timeframe()].PopLock() {
			n.processCandle(s.Timeframe(), item.(model.Candle))
		}
	}
}

// Start the backtest process and create a progress bar
// backtestCandles will process candles from a prirority queue in chronological order
func (n *Floolisher) backtestCandles() {
	log.Info("[SETUP] Starting backtesting")

	for _, s := range n.strategy.Strategies {
		progressBar := progressbar.Default(int64(n.priorityQueueCandles[s.Timeframe()].Len()))
		for n.priorityQueueCandles[s.Timeframe()].Len() > 0 {
			item := n.priorityQueueCandles[s.Timeframe()].Pop()

			candle := item.(model.Candle)
			if n.paperWallet != nil {
				n.paperWallet.OnCandle(candle)
			}

			n.strategiesControllers[candle.Pair].OnPartialCandle(candle)
			if candle.Complete {
				n.strategiesControllers[candle.Pair].OnCandle(s.Timeframe(), candle)
			}

			if err := progressBar.Add(1); err != nil {
				log.Warnf("update progressbar fail: %v", err)
			}
		}
	}
}

// Before Ninjabot start, we need to load the necessary data to fill strategy indicators
// Then, we need to get the time frame and warmup period to fetch the necessary candles
func (n *Floolisher) preload(ctx context.Context, pair string) error {
	if n.backtest {
		return nil
	}
	for _, s := range n.strategy.Strategies {
		candles, err := n.exchange.CandlesByLimit(ctx, pair, s.Timeframe(), s.WarmupPeriod())
		if err != nil {
			return err
		}

		for _, candle := range candles {
			n.processCandle(s.Timeframe(), candle)
		}
		n.dataFeed.Preload(pair, s.Timeframe(), candles)
	}

	return nil
}

// Run will initialize the strategy controller, order controller, preload data and start the bot
func (n *Floolisher) Run(ctx context.Context) error {
	for _, pair := range n.settings.Pairs {
		// setup and subscribe strategy to data feed (candles)
		n.strategiesControllers[pair] = handler.NewStrategyController(pair, n.strategy, n.orderController)

		// preload candles for warmup period
		err := n.preload(ctx, pair)
		if err != nil {
			return err
		}

		// link to ninja bot controller
		for _, s := range n.strategy.Strategies {
			n.dataFeed.Subscribe(pair, s.Timeframe(), n.onCandle, false)
		}

		// start strategy controller
		n.strategiesControllers[pair].Start()
	}

	// start order feed and controller
	n.orderFeed.Start()
	n.orderController.Start()
	defer n.orderController.Stop()
	if n.telegram != nil {
		n.telegram.Start()
	}

	// start data feed and receives new candles
	n.dataFeed.Start(n.backtest)

	// start processing new candles for production or backtesting environment
	if n.backtest {
		n.backtestCandles()
	} else {
		n.processCandles()
	}

	return nil
}
