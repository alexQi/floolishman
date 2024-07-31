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

	formatter := &logrus.TextFormatter{
		FullTimestamp:   true,
		DisableQuote:    true,
		TimestampFormat: "15:04:05",
	}
	Log.SetFormatter(formatter)
	Log.Out = os.Stdout

	err := loglevel.UnmarshalText([]byte(logLevel))
	if err != nil {
		loglevel = 4
		Log.Infof("Use default log Level: INFO")
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
		NewSimpleLogger(Log, dataPath, 30, formatter)
	}
	return Log
}

/*
*

	文件日志
*/
func NewSimpleLogger(log *logrus.Logger, logPath string, save uint, formatter logrus.Formatter) {

	lfHook := lfshook.NewHook(lfshook.WriterMap{
		logrus.DebugLevel: writer(logPath, "debug", save),
		logrus.TraceLevel: writer(logPath, "trace", save),
		logrus.InfoLevel:  writer(logPath, "info", save),
		logrus.WarnLevel:  writer(logPath, "warn", save),
		logrus.ErrorLevel: writer(logPath, "error", save),
		logrus.FatalLevel: writer(logPath, "fatal", save),
		logrus.PanicLevel: writer(logPath, "panic", save),
	}, formatter)
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
		rotatelogs.WithRotationTime(time.Second), // 日志切割时间间隔
		rotatelogs.WithMaxAge(-1),                // 关闭过期清理
		rotatelogs.WithRotationCount(int(save)),  // 文件最大保存份数
		//rotatelogs.WithLinkName(logFullPath+".out"), // 生成软链，指向最新日志文件
	)

	if err != nil {
		panic(err)
	}
	return logier
}
