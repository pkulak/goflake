package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type server struct {
	sync.Mutex

	fsm  *fsm
	peer *peer
	pool *pool
	addr string

	waiting int // how many outstanding requests are we waiting to be committed?
}

func newServer(c *Config) (*server, error) {
	fsmCh := make(chan idRange)
	fsm := newFSM(fsmCh)

	peer, err := newPeer(c, fsm)
	if err != nil {
		return nil, err
	}

	poolCh := make(chan uint64)
	pool := newPool(poolCh)

	svr := &server{fsm: fsm, peer: peer, pool: pool, addr: c.Addr}

	// apply requests for new batches
	go func() {
		for amount := range poolCh {
			svr.Lock()
			svr.waiting = svr.waiting + 1
			svr.Unlock()

			peer.apply(&action{Cmd: CmdRange, Amount: amount, Owner: peer.addr}, applyRetries)
		}
	}()

	// and when they come back, consume them
	go func() {
		for rng := range fsmCh {
			// if it's not ours, ignore it
			if rng.owner != peer.addr {
				continue
			}

			svr.Lock()

			// if we are not expecting it, ignore it (log replay)
			if svr.waiting < 1 {
				svr.Unlock()
				continue
			}

			svr.waiting = svr.waiting - 1
			svr.Unlock()

			pool.addRange(rng)
		}
	}()

	return svr, nil
}

func (s *server) start() {
	mux := http.NewServeMux()

	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%d", s.pool.getID())
	})

	mux.HandleFunc("/apply", func(w http.ResponseWriter, r *http.Request) {
		var a action
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		r.Body.Close()
		s.peer.apply(&a, applyRetries)
	})

	log.Info("starting http server on %v", s.addr)

	log.Fatal(http.ListenAndServe(s.addr, mux))
}
