package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-fuego/fuego"
	_ "github.com/mattn/go-sqlite3"
)

type Memory struct {
	ID        int       `json:"id"`
	MemoryID  string    `json:"memory_id"`
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SaveMemoryInput struct {
	MemoryID string `json:"memory_id"`
	Content  string `json:"content"`
}

type UpdateMemoryInput struct {
	MemoryID string `json:"memory_id"`
	Content  string `json:"content"`
}

type DeleteMemoryInput struct {
	MemoryID string `json:"memory_id"`
}

type StatusResponse struct {
	Status   string `json:"status"`
	MemoryID string `json:"memory_id"`
	Version  int    `json:"version,omitempty"`
}

var shutdownRequested atomic.Bool

func main() {
	fmt.Println("[DEBUG] Starting main()...")
	dsn := os.Getenv("MEMORY_SERVER_DSN")
	if dsn == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic("Could not determine user home directory")
		}
		dsn = home + "/Databases/memory_server.sqlite"
	}
	fmt.Printf("[DEBUG] Using DSN: %s\n", dsn)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		fmt.Printf("[DEBUG] sql.Open error: %v\n", err)
		panic(err)
	}
	defer db.Close()

	_, err = db.Exec(readSchema())
	if err != nil {
		fmt.Printf("[DEBUG] db.Exec(readSchema) error: %v\n", err)
		panic(err)
	}
	fmt.Println("[DEBUG] DB schema ensured.")

	s := fuego.NewServer()
	fmt.Println("[DEBUG] Fuego server created.")

	// Index
	fuego.Get(s, "/", func(c fuego.ContextNoBody) (string, error) {
		return "Windsurf Memory Server: See /openapi.json for API docs.", nil
	})

	// Save memory
	fuego.Post(s, "/save-memory", func(c fuego.ContextWithBody[SaveMemoryInput]) (*StatusResponse, error) {
		body, err := c.Body()
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Bad Request", Detail: err.Error()}
		}
		var version int
		err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM memories WHERE memory_id = ?", body.MemoryID).Scan(&version)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		version++
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO memories (memory_id, version, content, archived, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, body.MemoryID, version, body.Content, now, now)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		return &StatusResponse{Status: "saved", MemoryID: body.MemoryID, Version: version}, nil
	})

	// Update memory
	fuego.Post(s, "/update-memory", func(c fuego.ContextWithBody[UpdateMemoryInput]) (*StatusResponse, error) {
		body, err := c.Body()
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Bad Request", Detail: err.Error()}
		}
		_, err = db.Exec("UPDATE memories SET archived=1 WHERE memory_id=? AND archived=0", body.MemoryID)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		var version int
		err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM memories WHERE memory_id = ?", body.MemoryID).Scan(&version)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		version++
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO memories (memory_id, version, content, archived, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, body.MemoryID, version, body.Content, now, now)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		return &StatusResponse{Status: "updated", MemoryID: body.MemoryID, Version: version}, nil
	})

	// Delete memory (archive all)
	fuego.Post(s, "/delete-memory", func(c fuego.ContextWithBody[DeleteMemoryInput]) (*StatusResponse, error) {
		body, err := c.Body()
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Bad Request", Detail: err.Error()}
		}
		_, err = db.Exec("UPDATE memories SET archived=1 WHERE memory_id=?", body.MemoryID)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		return &StatusResponse{Status: "archived", MemoryID: body.MemoryID}, nil
	})

	// List memories (latest, not archived)
	fuego.Get(s, "/list-memories", func(c fuego.ContextNoBody) ([]Memory, error) {
		rows, err := db.Query(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE archived=0 ORDER BY memory_id, version DESC`)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		defer rows.Close()
		var memories []Memory
		for rows.Next() {
			var m Memory
			var archivedBool bool
			if err := rows.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
			}
			m.Archived = archivedBool
			memories = append(memories, m)
		}
		return memories, nil
	})

	// Get memory by id (latest, not archived)
	fuego.Get(s, "/get-memory-by-id/{memory_id}", func(c fuego.ContextNoBody) (*Memory, error) {
		memoryID := c.PathParam("memory_id")
		row := db.QueryRow(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE memory_id=? AND archived=0 ORDER BY version DESC LIMIT 1`, memoryID)
		var m Memory
		var archivedBool bool
		if err := row.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fuego.NotFoundError{Title: "Not Found", Detail: "not found"}
		}
		m.Archived = archivedBool
		return &m, nil
	})

	// Search memories (active only)
	fuego.Get(s, "/search-memories", func(c fuego.ContextNoBody) ([]Memory, error) {
		q := c.QueryParam("q")
		rows, err := db.Query(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE archived=0 AND (memory_id LIKE ? OR content LIKE ?) ORDER BY memory_id, version DESC`, "%"+q+"%", "%"+q+"%")
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
		}
		defer rows.Close()
		var memories []Memory
		for rows.Next() {
			var m Memory
			var archivedBool bool
			if err := rows.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: err.Error()}
			}
			m.Archived = archivedBool
			memories = append(memories, m)
		}
		return memories, nil
	})

	// Test-only shutdown endpoint
	fuego.Post(s, "/shutdown", func(c fuego.ContextNoBody) (string, error) {
		shutdownRequested.Store(true)
		return "shutting down", nil
	})

	fmt.Println("[DEBUG] Setting up HTTP server...")
	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: s.Mux,
	}

	// Graceful shutdown on signal or /shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			if shutdownRequested.Load() {
				fmt.Println("[DEBUG] /shutdown endpoint triggered, shutting down...")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				httpServer.Shutdown(ctx)
				return
			}
			select {
			case sig := <-quit:
				fmt.Printf("[DEBUG] Caught signal: %v, shutting down...\n", sig)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				httpServer.Shutdown(ctx)
				return
			case <-time.After(200 * time.Millisecond):
				// poll for shutdownRequested
			}
		}
	}()

	fmt.Println("[DEBUG] Calling httpServer.ListenAndServe()...")
	err = httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		fmt.Printf("[DEBUG] ListenAndServe error: %v\n", err)
		panic(err)
	}
	fmt.Println("[DEBUG] Server exited cleanly.")
}

func readSchema() string {
	return `CREATE TABLE IF NOT EXISTS memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		content TEXT NOT NULL,
		archived BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_memories_memory_id ON memories(memory_id);
	CREATE INDEX IF NOT EXISTS idx_memories_archived ON memories(archived);
	CREATE INDEX IF NOT EXISTS idx_memories_latest_active ON memories(memory_id, version, archived);`
}
