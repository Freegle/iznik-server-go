package sentrylogpackage

// Based on default logger but with Sentry capture added to init and error levels.

import (
	"context"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	logger2 "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"log"
	"time"
)

// ErrRecordNotFound record not found error
var ErrRecordNotFound = errors.New("record not found")

// Config logger config
type Config struct {
	SlowThreshold             time.Duration
	Colorful                  bool
	IgnoreRecordNotFoundError bool
	ParameterizedQueries      bool
	LogLevel                  logger2.LogLevel
}

// New initialize logger
func New(config Config) logger2.Interface {
	fmt.Println("Initializing Sentry")
	err := sentry.Init(sentry.ClientOptions{
		AttachStacktrace: true,
	})
	defer sentry.Flush(2 * time.Second)

	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}

	var (
		infoStr      = "%s\n[info] "
		warnStr      = "%s\n[warn] "
		errStr       = "%s\n[error] "
		traceStr     = "%s\n[%.3fms] [rows:%v] %s"
		traceWarnStr = "%s %s\n[%.3fms] [rows:%v] %s"
		traceErrStr  = "%s %s\n[%.3fms] [rows:%v] %s"
	)

	return &logger{
		Config:       config,
		infoStr:      infoStr,
		warnStr:      warnStr,
		errStr:       errStr,
		traceStr:     traceStr,
		traceWarnStr: traceWarnStr,
		traceErrStr:  traceErrStr,
	}
}

type logger struct {
	Config
	infoStr, warnStr, errStr            string
	traceStr, traceErrStr, traceWarnStr string
}

// LogMode log mode
func (l *logger) LogMode(level logger2.LogLevel) logger2.Interface {
	newlogger := *l
	newlogger.LogLevel = level
	return &newlogger
}

func (l logger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger2.Info {
		fmt.Printf(l.infoStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
	}
}

// Warn print warn messages
func (l logger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger2.Warn {
		fmt.Printf(l.warnStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
	}
}

// Error print error messages
func (l logger) Error(ctx context.Context, msg string, data ...interface{}) {
	fmt.Printf("ERROR")
	if l.LogLevel >= logger2.Error {
		fmt.Printf(l.errStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
		err := fmt.Errorf(l.errStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
		sentry.CaptureMessage(err.Error())
	}
}

// Trace print sql message
func (l logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if err != nil {
		fmt.Println("TRACE", err.Error())
		sentry.CaptureMessage(err.Error())
	}

	if l.LogLevel <= logger2.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.LogLevel >= logger2.Error && (!errors.Is(err, ErrRecordNotFound) || !l.IgnoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			fmt.Printf(l.traceErrStr, utils.FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			fmt.Printf(l.traceErrStr, utils.FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger2.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		if rows == -1 {
			fmt.Printf(l.traceWarnStr, utils.FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			fmt.Printf(l.traceWarnStr, utils.FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case l.LogLevel == logger2.Info:
		sql, rows := fc()
		if rows == -1 {
			fmt.Printf(l.traceStr, utils.FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			fmt.Printf(l.traceStr, utils.FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}
