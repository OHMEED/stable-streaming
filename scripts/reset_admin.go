package main

import (
	"database/sql"
	"fmt"
	"log"
    "os"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func main() {
    // Try to find data.db
    dbPath := "data.db"
    if _, err := os.Stat(dbPath); os.IsNotExist(err) {
        // Try parent directory
        if _, err := os.Stat("../data.db"); err == nil {
            dbPath = "../data.db"
        }
    }
    
    fmt.Printf("Using database at: %s\n", dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	password := "admin"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Check if admin exists
	var id int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'admin'").Scan(&id)
	if err == sql.ErrNoRows {
		// Insert
		_, err = db.Exec("INSERT INTO users (username, password_hash) VALUES ('admin', ?)", string(hash))
		if err != nil {
			log.Fatalf("Failed to insert admin user: %v", err)
		}
		fmt.Println("Admin user created with password 'admin'")
	} else if err == nil {
		// Update
		_, err = db.Exec("UPDATE users SET password_hash = ? WHERE username = 'admin'", string(hash))
		if err != nil {
			log.Fatalf("Failed to update admin password: %v", err)
		}
		fmt.Println("Admin password updated to 'admin'")
	} else {
		log.Fatalf("Database error: %v", err)
	}
}
