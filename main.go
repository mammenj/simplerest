package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync" // For basic concurrency safety on the database connection

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// Item represents the structure of our data
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var (
	db   *sql.DB
	dbMu sync.Mutex // Mutex to protect database operations
)

// initDB initializes the SQLite database and creates the 'items' table
func initDB(dataSourceName string) {
	var err error
	// For modernc.org/sqlite, the DSN is typically just the file path
	db, err = sql.Open("sqlite", dataSourceName) // Note: "sqlite" as driver name
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Ping the database to ensure the connection is established
	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Printf("Connected to SQLite database: %s", dataSourceName)

	// Create the 'items' table if it doesn't exist
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	);`

	dbMu.Lock()
	_, err = db.Exec(createTableSQL)
	dbMu.Unlock()
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	log.Println("Table 'items' ensured to exist.")
}

// getItemsHandler retrieves all items from the database
func getItemsHandler(w http.ResponseWriter, r *http.Request) {
	dbMu.Lock()
	rows, err := db.Query("SELECT id, name FROM items")
	dbMu.Unlock()
	if err != nil {
		http.Error(w, "Failed to retrieve items", http.StatusInternalServerError)
		log.Printf("Error querying items: %v", err)
		return
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			http.Error(w, "Failed to scan item", http.StatusInternalServerError)
			log.Printf("Error scanning item: %v", err)
			return
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error iterating rows", http.StatusInternalServerError)
		log.Printf("Error during row iteration: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// getItemByIDHandler retrieves a single item by its ID
func getItemByIDHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from the URL path using r.PathValue
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	var item Item
	dbMu.Lock()
	row := db.QueryRow("SELECT id, name FROM items WHERE id = ?", id)
	dbMu.Unlock()
	err = row.Scan(&item.ID, &item.Name)
	if err == sql.ErrNoRows {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to retrieve item", http.StatusInternalServerError)
		log.Printf("Error querying item by ID: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// createItemHandler creates a new item in the database
func createItemHandler(w http.ResponseWriter, r *http.Request) {
	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	dbMu.Lock()
	res, err := db.Exec("INSERT INTO items (name) VALUES (?)", item.Name)
	dbMu.Unlock()
	if err != nil {
		http.Error(w, "Failed to create item", http.StatusInternalServerError)
		log.Printf("Error inserting item: %v", err)
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		http.Error(w, "Failed to get last insert ID", http.StatusInternalServerError)
		log.Printf("Error getting last insert ID: %v", err)
		return
	}
	item.ID = int(id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

// updateItemHandler updates an existing item in the database
func updateItemHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from the URL path using r.PathValue
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	dbMu.Lock()
	res, err := db.Exec("UPDATE items SET name = ? WHERE id = ?", item.Name, id)
	dbMu.Unlock()
	if err != nil {
		http.Error(w, "Failed to update item", http.StatusInternalServerError)
		log.Printf("Error updating item: %v", err)
		return
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		http.Error(w, "Failed to get rows affected", http.StatusInternalServerError)
		log.Printf("Error getting rows affected: %v", err)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Item not found or no changes made", http.StatusNotFound)
		return
	}

	item.ID = id // Ensure the ID in the response matches the updated item's ID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// deleteItemHandler deletes an item from the database
func deleteItemHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from the URL path using r.PathValue
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	dbMu.Lock()
	res, err := db.Exec("DELETE FROM items WHERE id = ?", id)
	dbMu.Unlock()
	if err != nil {
		http.Error(w, "Failed to delete item", http.StatusInternalServerError)
		log.Printf("Error deleting item: %v", err)
		return
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		http.Error(w, "Failed to get rows affected", http.StatusInternalServerError)
		log.Printf("Error getting rows affected: %v", err)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204 No Content for successful deletion
}

func main() {
	// Initialize the database connection.
	initDB("api.db")
	defer func() {
		dbMu.Lock()
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
		dbMu.Unlock()
	}()

	// Create a new ServeMux
	mux := http.NewServeMux()

	// Register specific handlers for each HTTP method and path
	mux.HandleFunc("GET /items", getItemsHandler)
	mux.HandleFunc("POST /items", createItemHandler)
	mux.HandleFunc("GET /items/{id}", getItemByIDHandler)
	mux.HandleFunc("PUT /items/{id}", updateItemHandler)
	mux.HandleFunc("DELETE /items/{id}", deleteItemHandler)

	port := "0.0.0.0:8080"
	log.Printf("Server starting on port %s", port)
	// Pass the mux to ListenAndServe
	log.Fatal(http.ListenAndServe(port, mux))
}
