package log

import (
	"fmt"
	rotatelogs "github.com/lestrrat/go-file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"path"
	"time"
)

func InitLogger() *logrus.Logger {
	var loglevel logrus.Level

	logLevel := viper.GetString("log.level")
	Log := logrus.New()
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	Log.Out = os.Stdout

	err := loglevel.UnmarshalText([]byte(logLevel))
	if err != nil {
		Log.Panicf("未知的日志级别：%v", err)
	}
	Log.SetLevel(loglevel)

	logSwitch := viper.GetBool("log.stdout")
	if logSwitch == true {
		dataPath := viper.GetString("log.path")
		// 判断文件目录是否存在
		_, err := os.Stat(dataPath)
		if err != nil {
			err = os.MkdirAll(dataPath, os.ModePerm)
			if err != nil {
				Log.Panicf("mkdir error : %s", err.Error())
			}
		}
		NewSimpleLogger(Log, dataPath, 30)
	}
	return Log
}

/*
*

	文件日志
*/
func NewSimpleLogger(log *logrus.Logger, logPath string, save uint) {

	lfHook := lfshook.NewHook(lfshook.WriterMap{
		logrus.DebugLevel: writer(logPath, "debug", save),
		logrus.TraceLevel: writer(logPath, "trace", save),
		logrus.InfoLevel:  writer(logPath, "info", save),
		logrus.WarnLevel:  writer(logPath, "warn", save),
		logrus.ErrorLevel: writer(logPath, "error", save),
		logrus.FatalLevel: writer(logPath, "fatal", save),
		logrus.PanicLevel: writer(logPath, "panic", save),
	}, &logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.AddHook(lfHook)
}

/*
*
文件设置
*/
func writer(logPath string, level string, save uint) *rotatelogs.RotateLogs {
	var tempFileFlag string
	if flag := viper.GetString("log.flag"); flag != "" {
		tempFileFlag = flag + "-"
	}

	tempFileFlag += level
	logFullPath := path.Join(logPath, tempFileFlag)
	logFullPath = fmt.Sprintf("%s", logFullPath)

	logier, err := rotatelogs.New(
		logFullPath+"-%Y%m%d."+viper.GetString("log.suffix"),
		rotatelogs.WithRotationTime(time.Second),    // 日志切割时间间隔
		rotatelogs.WithMaxAge(-1),                   // 关闭过期清理
		rotatelogs.WithLinkName(logFullPath+".out"), // 生成软链，指向最新日志文件
		rotatelogs.WithRotationCount(int(save)),     // 文件最大保存份数
	)

	if err != nil {
		panic(err)
	}
	return logier
}
