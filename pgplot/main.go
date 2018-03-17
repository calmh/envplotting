package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", os.Getenv("CONNSTRING"))
	if err != nil {
		log.Fatal(err)
	}

	h := &handler{
		db: db,
	}

	http.HandleFunc("/stats/power", h.power)
	http.HandleFunc("/stats/temperature", h.temperature)
	log.Fatal(http.ListenAndServe(os.Getenv("LISTENADDR"), nil))
}

type handler struct {
	db *sql.DB
}

type datapoint struct {
	Timestamp time.Time `json:"timestamp"`
	WattHours int64     `json:"wh,omitempty"`
	DegreesC  float64   `json:"degc,omitempty"`
}

func (h *handler) power(w http.ResponseWriter, req *http.Request) {
	tpl := `SELECT hour, wh FROM (
	SELECT hour, maxwh-LAG(maxwh, 1) OVER () AS wh
	FROM (
		SELECT DATE_TRUNC('hour', ts) + INTERVAL '1 hour' AS hour, MAX(wh) AS maxwh
		FROM env
		WHERE
			DATE_TRUNC('hour', ts) > NOW() - INTERVAL '%d hours' AND
			DATE_TRUNC('hour', ts) < DATE_TRUNC('hour', NOW())
		GROUP BY hour
		ORDER BY hour ASC
	) AS subsel1
) AS subsel2
WHERE wh IS NOT NULL;`

	hours := 24
	if v, err := strconv.Atoi(req.URL.Query().Get("hours")); err == nil && v > 0 {
		hours = v
	}

	query := fmt.Sprintf(tpl, hours+2)
	var res []datapoint
	err := h.query(query, func(rows *sql.Rows) error {
		var p datapoint
		if err := rows.Scan(&p.Timestamp, &p.WattHours); err != nil {
			return err
		}
		res = append(res, p)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // actually the time until next hour
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(res)
}

func (h *handler) temperature(w http.ResponseWriter, req *http.Request) {
	tpl := `SELECT
	DATE_TRUNC('hour', ts) + DATE_PART('minute', ts)::int / %d * INTERVAL '%d min' AS fmts,
	ROUND(AVG(degc)::NUMERIC, 2) AS avgc
FROM env
WHERE ts > NOW() - INTERVAL '%d hours'
GROUP BY fmts
ORDER BY fmts ASC;`

	hours := 24
	if v, err := strconv.Atoi(req.URL.Query().Get("hours")); err == nil && v > 0 {
		hours = v
	}
	stepMinutes := 5
	if v, err := strconv.Atoi(req.URL.Query().Get("stepm")); err == nil && v > 0 {
		stepMinutes = v
	}
	query := fmt.Sprintf(tpl, stepMinutes, stepMinutes, hours)

	var res []datapoint
	err := h.query(query, func(rows *sql.Rows) error {
		var p datapoint
		if err := rows.Scan(&p.Timestamp, &p.DegreesC); err != nil {
			return err
		}
		res = append(res, p)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(res)
}

func (h *handler) query(qstr string, scanFn func(rows *sql.Rows) error) error {
	rows, err := h.db.Query(qstr)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scanFn(rows); err != nil {
			return err
		}
	}

	return nil
}

/*
SELECT hour, maxwh-LAG(maxwh, -1) OVER () AS wh
FROM (
	SELECT DATE_TRUNC('hour', ts) + INTERVAL '1 hour' AS hour, MAX(wh) AS maxwh
	FROM env
	WHERE
		DATE_TRUNC('hour', ts) > NOW() - INTERVAL '10 hours' AND
		DATE_TRUNC('hour', ts) < DATE_TRUNC('hour', NOW())
	GROUP BY hour
	ORDER BY hour DESC
) AS subsel
LIMIT 8;



SELECT hour, wh FROM (
	SELECT hour, maxwh-LAG(maxwh, -1) OVER () AS wh
	FROM (
		SELECT DATE_TRUNC('hour', ts) + INTERVAL '1 hour' AS hour, MAX(wh) AS maxwh
		FROM env
		WHERE
			DATE_TRUNC('hour', ts) > NOW() - INTERVAL '10 hours' AND
			DATE_TRUNC('hour', ts) < DATE_TRUNC('hour', NOW())
		GROUP BY hour
		ORDER BY hour DESC
	) AS subsel1
	ORDER BY hour
) AS subsel2
WHERE wh IS NOT NULL;
*/
