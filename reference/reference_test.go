package reference

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Performance test of the basic framework

func TestFrameworkPerformance(t *testing.T) {
	var wg sync.WaitGroup

	var times = 10

	start := time.Now()

	for i := 0; i < times; i++ {
		respChan := make(chan interface{}, 1)
		wg.Add(1)

		go func() {
			defer wg.Done()
			genAnswer(respChan)
			<-respChan
		}()
	}

	wg.Wait()

	end := time.Now()
	duration := end.Sub(start)
	timeNanos := duration.Nanoseconds()

	fmt.Printf("Nanoseconds per operation %d\n", timeNanos/int64(times))
}

func genAnswer(rChan chan interface{}) {
	rChan <- struct{}{}
	close(rChan)
}

func TestLog(t *testing.T) {

	defaultLogConfig := `{
		"level": "info",
		"development": true,
		"encoding": "console",
		"outputPaths": ["stdout"],
		"errorOutputPaths": ["stderr"],
		"disableCaller": false,
		"disableStackTrace": false,
		"encoderConfig": {
			"messageKey": "message",
			"levelKey": "level",
			"levelEncoder": "lowercase",
			"callerKey": "caller",
			"callerEncoder": "",
			"timeKey": "ts",
			"timeEncoder": "ISO8601"
			}
		}`

	// Parse the JSON
	var cfg zap.Config
	if err := json.Unmarshal([]byte(defaultLogConfig), &cfg); err != nil {
		panic("bad log configuration " + err.Error())
	}

	nOps := int64(1000000)

	startTime := time.Now()
	for i := 0; i < int(nOps); i++ {
		var b bytes.Buffer
		core := zapcore.NewCore(
			zapcore.NewConsoleEncoder(cfg.EncoderConfig),
			zapcore.AddSync(&b),
			zapcore.InfoLevel,
		)

		logger := zap.New(core).Sugar()
		logger.Infof("this is a log line with parameter %s", "parameter")
		logger.Sync()

		_ = fmt.Sprintf("%s", b.String())

		//fmt.Println(b.String())

	}
	endTime := time.Now()
	fmt.Printf("%d operations per second\n", 1000000*nOps/endTime.Sub(startTime).Microseconds())
}
