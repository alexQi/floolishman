package main

import (
	"floolishman/download"
	"floolishman/exchange"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils/strutil"
	"fmt"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"log"
	"os"
)

func main() {
	// 获取基础配置
	var (
		callerSetting = types.CallerSetting{
			CheckMode:   viper.GetString("caller.checkMode"),
			IgnorePairs: viper.GetStringSlice("caller.ignorePairs"),
			Leverage:    viper.GetInt("caller.leverage"),
		}
	)

	app := &cli.App{
		Name:     "floolishman",
		HelpName: "floolishman",
		Usage:    "Utilities for bot creation",
		Commands: []*cli.Command{
			{
				Name:     "download",
				HelpName: "download",
				Usage:    "Download historical data",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "pair",
						Aliases:  []string{"p"},
						Usage:    "eg. BTCUSDT",
						Required: false,
					},
					&cli.IntFlag{
						Name:     "days",
						Aliases:  []string{"d"},
						Usage:    "eg. 100 (default 30 days)",
						Required: false,
					},
					&cli.TimestampFlag{
						Name:     "start",
						Aliases:  []string{"s"},
						Usage:    "eg. 2021-12-01",
						Layout:   "2006-01-02",
						Required: false,
					},
					&cli.TimestampFlag{
						Name:     "end",
						Aliases:  []string{"e"},
						Usage:    "eg. 2020-12-31",
						Layout:   "2006-01-02",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "timeframe",
						Aliases:  []string{"t"},
						Usage:    "eg. 1h",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "output",
						Aliases:  []string{"o"},
						Usage:    "eg. ./btc.csv",
						Required: false,
					},
					&cli.BoolFlag{
						Name:     "futures",
						Aliases:  []string{"f"},
						Usage:    "true or false",
						Value:    false,
						Required: false,
					},
				},
				Action: func(c *cli.Context) error {
					var (
						exc reference.Feeder
						err error
					)

					if c.Bool("futures") {
						// fetch data from binance futures
						exc, err = exchange.NewBinanceFuture(c.Context)
						if err != nil {
							return err
						}
					} else {
						// fetch data from binance spot
						exc, err = exchange.NewBinance(c.Context)
						if err != nil {
							return err
						}
					}

					var options []download.Option
					if days := c.Int("days"); days > 0 {
						options = append(options, download.WithDays(days))
					}

					start := c.Timestamp("start")
					end := c.Timestamp("end")
					if start != nil && end != nil && !start.IsZero() && !end.IsZero() {
						options = append(options, download.WithInterval(*start, *end))
					} else if start != nil || end != nil {
						log.Fatal("START and END must be informed together")
					}
					var output string
					timeframe := c.String("timeframe")
					pair := c.String("pair")
					if len(pair) == 0 {
						//var wg sync.WaitGroup // 用于等待所有并发任务完成
						//sem := make(chan struct{}, 1)
						coinAssetInfos := exc.AssetsInfos()
						for pair, assetInfo := range coinAssetInfos {
							//wg.Add(1) // 增加WaitGroup计数器
							if strutil.ContainsString(callerSetting.IgnorePairs, pair) {
								continue
							}
							if assetInfo.QuoteAsset != "USDT" {
								continue
							}
							output = fmt.Sprintf("./testdata/%s-%s.csv", pair, c.String("timeframe"))
							err := download.NewDownloader(exc).Download(c.Context, pair, timeframe, output, options...)
							if err != nil {
								return err
							}

							//go func(pair string) {
							//	defer wg.Done() // 当goroutine完成时，减少WaitGroup计数器
							//	sem <- struct{}{}
							//	defer func() { <-sem }()
							//
							//	err := download.NewDownloader(exc).Download(c.Context, pair, timeframe, output, options...)
							//	if err != nil {
							//		return
							//	}
							//}(pair)
						}
						//wg.Wait()
						return nil
					} else {
						output = c.String("output")
						if len(output) == 0 {
							output = fmt.Sprintf("./testdata/%s-%s.csv", pair, timeframe)
						}
						return download.NewDownloader(exc).Download(c.Context, pair, timeframe, output, options...)
					}
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
