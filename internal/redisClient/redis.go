package redisClient

import (
	"github.com/go-redis/redis"
	"github.com/spf13/viper"
	"time"
)

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
	PoolSize int
}

func New() *redis.Client {
	redisConfig := RedisConfig{
		Host:     viper.GetString("redis.host"),
		Port:     viper.GetString("redis.port"),
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.database"),
		PoolSize: viper.GetInt("redis.poolsize"),
	}
	return redisConfig.Connect()
}

func (c *RedisConfig) Connect() *redis.Client {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     c.Host + ":" + c.Port,
		Password: c.Password,
		DB:       c.DB,

		// 连接池容量及闲置连接数量
		PoolSize:     c.PoolSize, // 连接池最大socket连接数，默认为4倍CPU数， 4 * runtime.NumCPU
		MinIdleConns: c.PoolSize, // 在启动阶段创建指定数量的Idle连接，并长期维持idle状态的连接数不少于指定数量；。

		// 超时
		DialTimeout:  10 * time.Second, // 连接建立超时时间，默认5秒。
		ReadTimeout:  10 * time.Second, // 读超时，默认3秒， -1表示取消读超时
		WriteTimeout: 10 * time.Second, // 写超时，默认等于读超时
		PoolTimeout:  30 * time.Second, // 当所有连接都处在繁忙状态时，客户端等待可用连接的最大等待时长，默认为读超时+1秒。

		// 闲置连接检查包括IdleTimeout，MaxConnAge
		IdleCheckFrequency: 60 * time.Second, // 闲置连接检查的周期，默认为1分钟，-1表示不做周期性检查，只在客户端获取连接时对闲置连接进行处理。
		IdleTimeout:        5 * time.Second,  // 闲置超时，默认5分钟，-1表示取消闲置超时检查
		MaxConnAge:         0 * time.Second,  // 连接存活时长，从创建开始计时，超过指定时长则关闭连接，默认为0，即不关闭存活时长较长的连接

		// 命令执行失败时的重试策略
		MaxRetries:      0,                      // 命令执行失败时，最多重试多少次，默认为0即不重试
		MinRetryBackoff: 8 * time.Millisecond,   // 每次计算重试间隔时间的下限，默认8毫秒，-1表示取消间隔
		MaxRetryBackoff: 512 * time.Millisecond, // 每次计算重试间隔时间的上限，默认512毫秒，-1表示取消间隔

		// 可自定义连接函数
		// Dialer: func() (net.Conn, error) {
		// 	netDialer := &net.Dialer{
		// 		Timeout:   5 * time.Second,
		// 		KeepAlive: 5 * time.Minute,
		// 	}
		// 	return netDialer.Dial("tcp", "127.0.0.1:6379")
		// },

		// 钩子函数
		// OnConnect: func(conn *redis.Conn) error { // 仅当客户端执行命令时需要从连接池获取连接时，如果连接池需要新建连接时则会调用此钩子函数
		// 	fmt.Printf("conn=%v\n", conn)
		// 	return nil
		// },
	})
	return redisClient
}
