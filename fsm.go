package main

import (
	"sync"
)

const (
	CmdRange     = "range"
	CmdNewLeader = "new_leader"
)

// Our finite state machine that gets synced between all peers
type fsm struct {
	sync.Mutex

	Start uint64 `json:"start"` // the start of the range (inclusive)
	End   uint64 `json:"end"`   // the end of the range (exclusive)
	Owner string `json:"owner"` // the owning peer of this range

	leader       string       `json:"-"` // keep track of the http address of the leader
	newRangeChan chan idRange `json:"-"` // used to notify when a new range has been committed
}

func newFSM(newRangeChan chan idRange) *fsm {
	return &fsm{newRangeChan: newRangeChan}
}

func (fsm *fsm) addRange(amount uint64, owner string) {
	fsm.Start = fsm.End
	fsm.End = fsm.Start + amount
	fsm.Owner = owner
}

func (fsm *fsm) newLeader(leader string) {
	fsm.leader = leader
}

type action struct {
	Cmd    string `json:"cmd"`
	Leader string `json:"leader"`
	Amount uint64 `json:"amount"`
	Owner  string `json:"owner"`
}

func (fsm *fsm) handleAction(a *action) {
	fsm.Lock()
	defer fsm.Unlock()

	switch a.Cmd {
	case CmdRange:
		fsm.addRange(a.Amount, a.Owner)
		fsm.newRangeChan <- newIDRange(fsm.Start, fsm.End, fsm.Owner)
	case CmdNewLeader:
		fsm.newLeader(a.Leader)
	}
}
