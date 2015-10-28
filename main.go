package main

import (
	"flag"
	"fmt"
	"github.com/op/go-logging"
	"os"
)

var log = logging.MustGetLogger("goflake")

var logFormat = logging.MustStringFormatter(
	"%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}",
)

var configFile = flag.String("config", "", "TOML config file")

func main() {
	// set up logging
	logging.SetBackend(logging.NewBackendFormatter(logging.NewLogBackend(os.Stderr, "", 0), logFormat))

	// grab our config
	flag.Parse()

	var c *Config
	var err error

	if len(*configFile) > 0 {
		c, err = NewConfigWithFile(*configFile)
		if err != nil {
			fmt.Printf("load failover config %s err %v\n", *configFile, err)
			return
		}
	} else {
		fmt.Printf("No config file.\n")
		fmt.Printf("Please specify with the --config option.\n")
		return
	}

	// and start up the ol' HTTP server
	s, err := newServer(c)
	if err != nil {
		log.Fatalf("could not start http server: %v", err)
	}

	s.start()
}
