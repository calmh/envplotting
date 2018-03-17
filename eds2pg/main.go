package main

import (
	"log"
	"os"
	"time"

	"github.com/thejerf/suture"
)

type datapoint struct {
	time        time.Time
	temperature float64
	wattHours   int64
}

var interval = time.Minute
var debug = os.Getenv("EDSDEBUG") != ""

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	edsURL := os.Getenv("EDSURL")
	connstring := os.Getenv("CONNSTRING") // postgres://pqgotest:password@localhost/pqgotest?sslmode=verify-full

	results := make(chan datapoint)
	pSrv := &poster{
		connstring: connstring,
		in:         results,
	}
	rSrv := &reader{
		url:  edsURL,
		out:  results,
		intv: interval,
	}

	srv := suture.NewSimple("main")
	srv.Add(pSrv)
	srv.Add(rSrv)
	srv.Serve()
}
