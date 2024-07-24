package serv

import (
	"floolishman/grpc/handler"
	"floolishman/model"
	"floolishman/pbs/guider"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"net"
)

// StartGrpcServer
func StartGrpcServer(
	guiderConfigs map[string]map[string]string,
	pairOptions []model.PairOption,
	proxyOption types.ProxyOption,
	st storage.Storage,
) {
	// 监听本地端口
	listener, err := net.Listen("tcp", viper.GetString("listen.grpc"))
	if err != nil {
		utils.Log.Panicf("net.Listen err: %v", err)
	}
	utils.Log.Infof("server started：Real-Time GRPC , listen on http://%s", viper.GetString("listen.grpc"))
	// 新建gRPC服务器实例
	options := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(208290415),
		grpc.MaxSendMsgSize(208290415),
	}
	grpcServer := grpc.NewServer(options...)
	// 获取guider handler
	handlerGuider := handler.NewGuiderHandler(guiderConfigs, pairOptions, proxyOption, st)
	// 在gRPC服务器注册我们的服务
	guider.RegisterGuiderWatcherServer(grpcServer, handlerGuider)
	// 用服务器 Serve() 方法以及我们的端口信息区实现阻塞等待，直到进程被杀死或者 Stop() 被调用
	err = grpcServer.Serve(listener)
	if err != nil {
		utils.Log.Fatalf("GRPC Server err: %v", err)
	}
}
