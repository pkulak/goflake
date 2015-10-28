package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
)

type integrityTester struct {
	sync.Mutex

	lastValue uint64
	lastTime  time.Time
	seen      []uint64
}

func newIntegrityTester() *integrityTester {
	return &integrityTester{
		lastValue: 0,
		lastTime:  time.Now(),
		seen:      make([]uint64, 0),
	}
}

func (it *integrityTester) check(val uint64) error {
	now := time.Now()

	it.Lock()
	defer it.Unlock()

	if val < it.lastValue {
		// this is okay, as long as it's not more than a second in the past.
		if it.lastTime.Before(now.Add(-1 * time.Second)) {
			return fmt.Errorf("value %v before %v and more than a second in that past (%v)", val, it.lastValue, now.Sub(it.lastTime))
		}
	}

	for _, i := range it.seen {
		if i == val {
			return fmt.Errorf("already seen %v", val)
		}
	}

	it.lastValue = val
	it.lastTime = now
	it.seen = append(it.seen, val)

	return nil
}

func newTestPool(delay time.Duration) *pool {
	notify := make(chan uint64)
	pool := newPool(notify)
	i := uint64(1000)
	var mutex sync.Mutex

	go func() {
		for size := range notify {
			time.Sleep(delay)

			mutex.Lock()
			start := i
			i = i + size
			mutex.Unlock()

			pool.addRange(newIDRange(start, start+size, ""))
		}
	}()

	return pool
}

func getRange(start uint64, end uint64, pool *pool, t *testing.T) {
	for i := uint64(start); i < end; i++ {
		id := pool.getID()

		if i != id {
			t.Errorf("invalid id %v, expected %v", id, i)
		}
	}
}

func getTicker(delay int32, amount int) chan bool {
	ticker := make(chan bool, amount)

	// do this in it's own thread to mimic the real world a bit better.
	go func() {
		for i := 0; i < amount; i++ {
			time.Sleep(time.Duration(rand.Int31n(delay)) * time.Millisecond)
			ticker <- true
		}
		close(ticker)
	}()

	return ticker
}

func getAmount(amount int, pool *pool, t *testing.T) {
	var last uint64

	for range getTicker(25, amount) {
		id := pool.getID()

		if id <= last {
			t.Error("got the same id twice!")
		}

		last = id
	}
}

func TestAddAndConsumeRange(t *testing.T) {
	pool := newTestPool(0)

	pool.addRange(newIDRange(100, 110, ""))
	getRange(100, 110, pool, t)
}

func TestAddAndConsumeMultipleRanges(t *testing.T) {
	pool := newTestPool(0)

	pool.addRange(newIDRange(100, 105, ""))
	pool.addRange(newIDRange(150, 155, ""))
	pool.addRange(newIDRange(175, 180, ""))

	getRange(100, 105, pool, t)
	getRange(150, 155, pool, t)
	getRange(175, 180, pool, t)
}

func TestAutoAddRanges(t *testing.T) {
	pool := newTestPool(100 * time.Millisecond)

	// just go!
	getAmount(1000, pool, t)
}

func TestMultipleThreads(t *testing.T) {
	// just make super-sure we're in a multi-threaded environmet
	runtime.GOMAXPROCS(runtime.NumCPU())

	tester := newIntegrityTester()

	pool := newTestPool(100)
	threads := runtime.NumCPU()

	done := make(chan bool)

	run := func() {
		for range getTicker(500, 50) {
			if err := tester.check(pool.getID()); err != nil {
				t.Error(err.Error())
			}
		}

		done <- true
	}

	for i := 0; i < threads; i++ {
		go run()
	}

	for i := 0; i < threads; i++ {
		<-done
	}
}
