package main

import (
	"database/sql"
	"flag"
	"fmt"
	"go-crawler/internal/crawler"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var workers int

	flag.IntVar(&workers, "workers", 1, "Number of worker")
	flag.Parse()

	db, err := sql.Open("sqlite3", "./crawler.db?_busy_timeout=1000&_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	db.SetMaxOpenConns(20) // max open connections
	db.SetMaxIdleConns(10) // how many idle ones can stick around

	defer db.Close()

	sqlBytes, err := os.ReadFile("schema.sql")
	if err != nil {
		log.Fatalf("Failed to read SQL schema: %v", err)
	}
	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	fmt.Printf("[INFO] Starting %d cralwer(s)\n", workers)
	crawler.Start(workers, db)
}
