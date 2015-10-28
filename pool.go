package main

import (
	"github.com/artyom/metrics"
	"math"
	"sync"
	"time"
)

type idRange struct {
	owner   string
	expires time.Time // when this range can no longer be distributed
	start   uint64    // inclusive
	end     uint64    // exclusive
}

func newIDRange(start uint64, end uint64, owner string) idRange {
	return idRange{
		expires: time.Now().Add(time.Second),
		start:   start,
		end:     end, owner: owner,
	}
}

func (rng idRange) isValid() bool {
	return rng.start > 0 && rng.end > rng.start
}

func (rng idRange) size() uint64 {
	return rng.end - rng.start
}

type pool struct {
	sync.Mutex

	ranges       []idRange    // ranges we're still waiting to distribute
	out          chan uint64  // used internally to hand out ids
	notify       chan uint64  // used to notify that we need another range
	distributing bool         // are we currently handing out ids?
	restart      chan bool    // receives when we should request a new range
	rate         metrics.EWMA // keep track of how much traffic we're getting
	ticked       bool         // have we measured the rate yet?
}

func newPool(notify chan uint64) *pool {
	p := &pool{
		ranges:  make([]idRange, 0, 2),
		out:     make(chan uint64),
		restart: make(chan bool),
		notify:  notify,
		rate:    metrics.NewEWMA1(),
	}

	// tick our metrics
	go func() {
		for _ = range time.Tick(metrics.TickDuration) {
			p.rate.Tick()
			p.ticked = true
		}
	}()

	// get this party started!
	go func() {
		p.restart <- true
	}()

	return p
}

func (pool *pool) nextRangeSize() uint64 {
	r := pool.rate.Rate()

	switch {
	case !pool.ticked:
		// if we haven't ticked yet, default to 10
		return 10
	case r < 1:
		return 1
	case r < 4:
		return uint64(r)
	default:
		return uint64(r * 0.75)
	}

	if pool.rate.Rate() == 0 {

		return 10
	} else {
		return uint64(math.Max(1, pool.rate.Rate()))
	}
}

func (pool *pool) addRange(rng idRange) {
	log.Info("got a new range %v", rng.size())

	pool.Lock()
	defer pool.Unlock()

	if !pool.distributing {
		pool.distributing = true

		go func() {
			pool.distributeRange(rng)
		}()
	} else {
		pool.ranges = append(pool.ranges, rng)
	}
}

func (pool *pool) distributeRange(rng idRange) {
	threshold := rng.start + uint64(float64(rng.end-rng.start)*0.9)
	requested := false

RangeLoop:
	for i := rng.start; i < rng.end; i++ {
		if time.Now().After(rng.expires) {
			log.Info("range expired top!")
			break
		}

		// I'm not totally sure what to do about this area. In theory, the
		// range could expire right here, after the above check, but before
		// we create the timer. That would mean it waits for the next get,
		// which could be an undetermined point in the future, and screws with
		// our gaurantees.
		select {
		case <-time.NewTimer(rng.expires.Sub(time.Now())).C:
			log.Info("range expired!")
			break RangeLoop
		case pool.out <- i:
		}

		if i == threshold && rng.size() > 2 {
			pool.Lock()

			if len(pool.ranges) == 0 {
				// we're about to run out! request some more
				log.Info("requesting another range")
				requested = true
				pool.notify <- pool.nextRangeSize()
			}

			pool.Unlock()
		}

		// update our rate
		pool.rate.Update(1)
	}

	// make sure there's nothing left in the queue
	var next idRange

	pool.Lock()

	if len(pool.ranges) > 0 {
		next, pool.ranges = pool.ranges[0], pool.ranges[1:] // shift
	}

	pool.distributing = next.isValid()
	pool.Unlock()

	if pool.distributing {
		pool.distributeRange(next)
	} else if requested == false {
		pool.restart <- true
	}
}

// where consumers wait on the next id
func (pool *pool) getID() uint64 {
	select {
	case id := <-pool.out:
		return id
	case <-pool.restart:
		pool.notify <- pool.nextRangeSize()
		return pool.getID()
	}
}
