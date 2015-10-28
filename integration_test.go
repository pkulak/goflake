package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"testing"
)

const (
	servers = 5
)

func makeRaftAddr(server int) string {
	return fmt.Sprintf("127.0.0.1:%d", 6346+server)
}

func makeHttpAddr(server int) string {
	return fmt.Sprintf("127.0.0.1:%d", 8080+server)
}

func makeDataDir(server int) string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err.Error())
	}

	return fmt.Sprintf("%s/goflake-data/%d", dir, server+1)
}

// fire up five servers and hit 'em
func TestIntegration(t *testing.T) {
	// just make super-sure we're in a multi-threaded environmet
	runtime.GOMAXPROCS(runtime.NumCPU())

	tester := newIntegrityTester()
	cluster := make([]string, 0, servers)

	for i := 0; i < servers; i++ {
		cluster = append(cluster, makeRaftAddr(i))
	}

	// start up all our servers
	for i := 0; i < servers; i++ {
		cfg := &Config{
			Addr: makeHttpAddr(i),
			Raft: RaftConfig{
				Addr:         makeRaftAddr(i),
				DataDir:      makeDataDir(i),
				Cluster:      cluster,
				ClusterState: ClusterStateNew,
			},
		}

		s, err := newServer(cfg)
		if err != nil {
			panic(err.Error())
		}

		go s.start()
	}

	done := make(chan bool)

	run := func() {
		defer func() {
			done <- true
		}()

		for range getTicker(100, 1000) {
			resp, err := http.Get("http://" + makeHttpAddr(int(rand.Int31n(servers))) + "/next")
			if err != nil {
				t.Error(err.Error())
				continue
			}

			if resp.StatusCode != 200 {
				t.Error("response not 200")
				continue
			}

			content, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Error("could not read response body")
				resp.Body.Close()
				continue
			}

			id, err := strconv.ParseUint(string(content), 10, 64)
			if err != nil {
				t.Error("could not parse response body: %v", string(content))
				resp.Body.Close()
				continue
			}

			if err := tester.check(id); err != nil {
				t.Error(err.Error())
			}

			resp.Body.Close()
		}
	}

	// hit them from different threads
	for i := 0; i < runtime.NumCPU(); i++ {
		go run()
	}

	// wait for them to finish
	for i := 0; i < runtime.NumCPU(); i++ {
		<-done
	}
}
