package serv

import (
	_ "floolishman/api"
	"floolishman/api/middlewares"
	"floolishman/api/routes"
	"floolishman/utils"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/recover"
	"github.com/spf13/viper"
)

func StartHttpServer() {
	app := iris.New()
	// 追加运行日志文件
	app.Logger().SetLevel(viper.GetString("log.level"))
	// 加载跨域中间件
	app.Use(middlewares.CorsNew())
	// 加载recover
	app.Use(recover.New())
	// 加载路由
	routes.ApiRoutes(app)
	// 获取iris config
	cfg := iris.DefaultConfiguration()
	err := viper.Unmarshal(&cfg)
	if err != nil {
		utils.Log.Errorf("unmarshal config failed: %s", err.Error())
	}
	// run iris
	err = app.Run(iris.Addr(viper.GetString("listen.http")), iris.WithConfiguration(cfg))
	if err != nil {
		utils.Log.Errorf(err.Error())
	}
}
