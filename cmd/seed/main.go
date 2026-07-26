package main

import (
	"fmt"

	"bengkel/database/seeders"
	"bengkel/internal/config"
	"bengkel/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	db, err := database.Open(cfg.DatabaseURL, false)
	if err != nil {
		panic(err)
	}
	if err := seeders.Run(db); err != nil {
		panic(err)
	}
	fmt.Println("DatabaseSeeder selesai.\nLogin demo: owner@bengkel.local / Bengkel123!")
}
