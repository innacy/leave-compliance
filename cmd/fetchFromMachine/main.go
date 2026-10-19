package main

import (
	"log"
	"os"

	fetchfrommachine "leave-compliance/scripts/fetchFromMachine"
)

func main() {
	log.Println("starting fetchFromMachine...")

	cfg, err := fetchfrommachine.LoadConfig()
	if err != nil {
		log.Printf("ERROR: configuration: %v", err)
		os.Exit(1)
	}
	log.Printf("config loaded: ESSL_URL=%s, API_URL=%s, DAYS_TO_FETCH=%d", cfg.ESSLURL, cfg.APIURL, cfg.DaysToFetch)

	if err := fetchfrommachine.Execute(cfg); err != nil {
		log.Printf("ERROR: %v", err)
		os.Exit(1)
	}

	log.Println("fetchFromMachine completed successfully")
}
