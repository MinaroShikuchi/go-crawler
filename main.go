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

	db, err := sql.Open("sqlite3", "./crawler.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	sqlBytes, err := os.ReadFile("schema.sql")
	if err != nil {
		log.Fatalf("Failed to read SQL schema: %v", err)
	}
	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	fmt.Printf("Starting cralwer %d\n", workers)
	crawler.Start(workers, db)
}
