package main

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

type post struct {
	Name    string          `json:"name"`
	Columns []string        `json:"columns"`
	Points  [][]interface{} `json:"points"`
}

type poster struct {
	connstring string
	db         *sql.DB
	in         <-chan datapoint

	stop chan struct{}
	lock sync.Mutex
}

func (p *poster) Serve() {
	p.lock.Lock()
	p.stop = make(chan struct{})
	p.lock.Unlock()

	log.Println(p, "starting")
	defer log.Println(p, "exiting")

	db, err := sql.Open("postgres", p.connstring)
	if err != nil {
		log.Println("db:", err)
		return
	}
	p.db = db

	var buffer []datapoint
	for {
		select {
		case data, ok := <-p.in:
			if !ok {
				return
			}
			buffer = append(buffer, data)
			err := p.post(buffer)
			if err != nil {
				log.Println("post:", err, "(buffering)")
			} else {
				buffer = nil
			}

		case <-p.stop:
			return
		}
	}
}

func (p *poster) Stop() {
	p.lock.Lock()
	close(p.stop)
	p.lock.Unlock()
}

func (p *poster) String() string {
	return fmt.Sprintf("poster@%p", p)
}

func (p *poster) post(buffer []datapoint) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	for _, dp := range buffer {
		_, err := tx.Exec("INSERT INTO env (ts, wh, degc) VALUES ($1, $2, $3)", dp.time.Truncate(time.Minute), dp.wattHours, dp.temperature)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}
