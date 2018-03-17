package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"launchpad.net/xmlpath"
)

type reader struct {
	url  string
	out  chan<- datapoint
	intv time.Duration

	stop chan struct{}
	lock sync.Mutex
}

func (r *reader) Serve() {
	r.lock.Lock()
	r.stop = make(chan struct{})
	r.lock.Unlock()

	log.Println(r, "starting")
	defer log.Println(r, "exiting")

	t := newSyncedTicker(r.intv)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			dp, err := parseURL(r.url)
			if err != nil {
				log.Println(err, "(fatal)")
				return
			}
			r.out <- dp

		case <-r.stop:
			return
		}
	}
}

func (r *reader) Stop() {
	r.lock.Lock()
	close(r.stop)
	r.lock.Unlock()
}

func (r *reader) String() string {
	return fmt.Sprintf("reader@%p", r)
}

func parseURL(url string) (datapoint, error) {
	client := http.Client{
		Timeout: 30 * time.Second,
	}

	var err error
	var resp *http.Response
	for i := 0; i < 5; i++ {
		resp, err = client.Get(url)
		if err == nil && resp.StatusCode != 200 {
			err = errors.New(resp.Status)
		}
		if err != nil {
			log.Println("get:", err, "(retrying)")
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		defer resp.Body.Close()
		return parseXML(resp.Body), nil
	}
	return datapoint{}, err
}

func parseXML(fd io.Reader) datapoint {
	root, err := xmlpath.Parse(fd)
	if err != nil {
		log.Fatal(err)
	}

	result := datapoint{
		time: time.Now(),
	}
	families := xmlpath.MustCompile("/Devices-Detail-Response/*[Name]")
	namePath := xmlpath.MustCompile("Name")
	iter := families.Iter(root)

	for iter.Next() {
		name, ok := namePath.String(iter.Node())
		if !ok {
			continue
		}
		switch name {
		case "DS18B20":
			// Thermometer
			strVal, _ := xmlpath.MustCompile("Temperature").String(iter.Node())
			result.temperature, _ = strconv.ParseFloat(strVal, 64)

		case "DS2423":
			// Counter
			strVal, _ := xmlpath.MustCompile("Counter_A").String(iter.Node())
			result.wattHours, _ = strconv.ParseInt(strVal, 10, 64)
		}
	}

	return result
}

type syncedTicker struct {
	intv time.Duration
	stop chan struct{}
	C    chan time.Time
}

func newSyncedTicker(intv time.Duration) *syncedTicker {
	t := &syncedTicker{
		intv: intv,
		stop: make(chan struct{}),
		C:    make(chan time.Time),
	}
	go t.tick()
	return t
}

func (t *syncedTicker) Stop() {
	close(t.stop)
}

func (t *syncedTicker) tick() {
	for {
		now := time.Now()
		next := now.Truncate(t.intv).Add(t.intv)
		sleep := next.Sub(now)
		time.Sleep(sleep)
		select {
		case t.C <- time.Now():
		case <-t.stop:
			return
		}
	}
}
