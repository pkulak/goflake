package main

import (
	"bytes"
	"encoding/json"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"io"
	"math"
	"net/http"
	"os"
	"path"
	"time"
)

const (
	applyRetries = 10
)

func (fsm *fsm) Apply(l *raft.Log) interface{} {
	var a action
	if err := json.Unmarshal(l.Data, &a); err != nil {
		log.Error("could not unmarshal raft log: %v", err)
		return err
	}

	fsm.handleAction(&a)
	return nil
}

func (fsm *fsm) Snapshot() (raft.FSMSnapshot, error) {
	return fsm, nil
}

func (fsm *fsm) Restore(data io.ReadCloser) error {
	fsm.Lock()
	defer fsm.Unlock()
	defer data.Close()

	d := json.NewDecoder(data)

	if err := d.Decode(fsm); err != nil {
		return err
	}

	return nil
}

func (fsm *fsm) Persist(sink raft.SnapshotSink) error {
	fsm.Lock()
	defer fsm.Unlock()

	data, _ := json.Marshal(fsm)
	_, err := sink.Write(data)
	if err != nil {
		sink.Cancel()
	}
	return err
}

func (fsm *fsm) Release() {}

type peer struct {
	r *raft.Raft

	dbStore   *raftboltdb.BoltStore
	trans     *raft.NetworkTransport
	peerStore *raft.JSONPeers
	fsm       *fsm

	addr string
}

func newPeer(c *Config, fsm *fsm) (*peer, error) {
	r := &peer{}
	var err error = nil

	r.addr = c.Raft.Addr

	os.MkdirAll(c.Raft.DataDir, 0755)

	cfg := raft.DefaultConfig()

	raftDBPath := path.Join(c.Raft.DataDir, "raft_db")
	r.dbStore, err = raftboltdb.NewBoltStore(raftDBPath)
	if err != nil {
		return nil, err
	}

	fileStore, err := raft.NewFileSnapshotStore(c.Raft.DataDir, 1, os.Stderr)
	if err != nil {
		return nil, err
	}

	r.trans, err = raft.NewTCPTransport(r.addr, nil, 3, 5*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	r.peerStore = raft.NewJSONPeers(c.Raft.DataDir, r.trans)

	if c.Raft.ClusterState == ClusterStateNew {
		log.Info("cluster state is new, use new cluster config")
		r.peerStore.SetPeers(c.Raft.Cluster)
	} else {
		log.Info("cluster state is existing, use previous + new cluster config")
		ps, err := r.peerStore.Peers()
		if err != nil {
			log.Error("get store peers error %v", err)
			return nil, err
		}

		for _, peer := range c.Raft.Cluster {
			ps = raft.AddUniquePeer(ps, peer)
		}

		r.peerStore.SetPeers(ps)
	}

	if peers, _ := r.peerStore.Peers(); len(peers) <= 1 {
		cfg.EnableSingleNode = true
		log.Notice("raft running in single node mode")
	}

	r.fsm = fsm

	r.r, err = raft.NewRaft(cfg, fsm, r.dbStore, r.dbStore, fileStore, r.peerStore, r.trans)
	if err != nil {
		return nil, err
	}

	// watch for leadership changes
	go func() {
		for isLeader := range r.r.LeaderCh() {
			if isLeader {
				log.Info("new leader http: %v", c.Addr)
				r.apply(&action{Cmd: CmdNewLeader, Leader: c.Addr}, applyRetries)
			}
		}
	}()

	return r, nil
}

func (r *peer) isLeader() bool {
	addr := r.r.Leader()
	return addr != "" && addr == r.addr
}

func (r *peer) apply(a *action, retries int) {
	if retries == 0 {
		log.Critical("too many retries trying to apply %v", a)
		return
	}

	if retries < applyRetries {
		sleep := time.Duration(math.Pow(float64((applyRetries-retries)*10), 2)) * time.Millisecond
		log.Error("retrying %v after %v", a, sleep)
		time.Sleep(sleep)
	}

	data, _ := json.Marshal(a)

	if r.isLeader() {
		r.r.Apply(data, 0)
	} else {
		resp, err := http.Post("http://"+r.fsm.leader+"/apply", "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Error(err.Error())
			r.apply(a, retries-1)
		} else if resp.StatusCode != 200 {
			log.Error("http apply status not 200: %v", resp.StatusCode)
			r.apply(a, retries-1)
		}
	}
}

func (r *peer) Close() {
	if r.trans != nil {
		r.trans.Close()
	}

	if r.r != nil {
		future := r.r.Shutdown()
		if err := future.Error(); err != nil {
			log.Error("Error shutting down raft: %v", err)
		}
	}

	if r.dbStore != nil {
		r.dbStore.Close()
	}
}
