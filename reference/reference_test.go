package reference

import (
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestSomething(t *testing.T) {

	var variable string = "hello"
	var itf interface{} = variable

	otherVariable, _ := itf.(string)

	variable = "modifiedhello"
	fmt.Println(variable, otherVariable)
}
