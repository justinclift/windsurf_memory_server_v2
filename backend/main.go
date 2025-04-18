package main

import (
	"database/sql"
	"net/http"
	"os"
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

func main() {
	dsn := os.Getenv("MEMORY_SERVER_DSN")
	if dsn == "" {
		dsn = "./memories.db"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	_, err = db.Exec(readSchema())
	if err != nil {
		panic(err)
	}

	r := fuego.NewRouter()
	r.Info.Title = "Windsurf Memory Server API"
	r.Info.Version = "1.0"
	r.Info.Description = "API for storing and managing versioned memories."

	r.GET("/", func(c *fuego.Context) error {
		return c.String(http.StatusOK, "Windsurf Memory Server: See /openapi.json for API docs.")
	})

	r.POST("/save-memory").Summary("Save a new memory").RequestBody(Memory{}).ResponseBody(Memory{}).Handler(func(c *fuego.Context) error {
		var req struct {
			MemoryID string `json:"memory_id"`
			Content  string `json:"content"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, fuego.H{"error": err.Error()})
		}
		var version int
		err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM memories WHERE memory_id = ?", req.MemoryID).Scan(&version)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		version++
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO memories (memory_id, version, content, archived, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, req.MemoryID, version, req.Content, now, now)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, fuego.H{"status": "saved", "memory_id": req.MemoryID, "version": version})
	})

	r.POST("/update-memory").Summary("Update an existing memory").RequestBody(Memory{}).ResponseBody(Memory{}).Handler(func(c *fuego.Context) error {
		var req struct {
			MemoryID string `json:"memory_id"`
			Content  string `json:"content"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, fuego.H{"error": err.Error()})
		}
		_, err := db.Exec("UPDATE memories SET archived=1 WHERE memory_id=? AND archived=0", req.MemoryID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		var version int
		err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM memories WHERE memory_id = ?", req.MemoryID).Scan(&version)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		version++
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO memories (memory_id, version, content, archived, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, req.MemoryID, version, req.Content, now, now)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, fuego.H{"status": "updated", "memory_id": req.MemoryID, "version": version})
	})

	r.POST("/delete-memory").Summary("Archive all versions of a memory").RequestBody(Memory{}).Handler(func(c *fuego.Context) error {
		var req struct {
			MemoryID string `json:"memory_id"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, fuego.H{"error": err.Error()})
		}
		_, err := db.Exec("UPDATE memories SET archived=1 WHERE memory_id=?", req.MemoryID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, fuego.H{"status": "archived", "memory_id": req.MemoryID})
	})

	r.GET("/list-memories").Summary("List all active (not archived) memories").ResponseBody([]Memory{}).Handler(func(c *fuego.Context) error {
		rows, err := db.Query(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE archived=0 ORDER BY memory_id, version DESC`)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		defer rows.Close()
		var memories []Memory
		for rows.Next() {
			var m Memory
			var archivedBool bool
			if err := rows.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
			}
			m.Archived = archivedBool
			memories = append(memories, m)
		}
		return c.JSON(http.StatusOK, memories)
	})

	r.GET("/get-memory-by-id/{memory_id}").Summary("Get the latest active version of a memory by memory_id").ResponseBody(Memory{}).Handler(func(c *fuego.Context) error {
		memoryID := c.Param("memory_id")
		row := db.QueryRow(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE memory_id=? AND archived=0 ORDER BY version DESC LIMIT 1`, memoryID)
		var m Memory
		var archivedBool bool
		if err := row.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return c.JSON(http.StatusNotFound, fuego.H{"error": "not found"})
		}
		m.Archived = archivedBool
		return c.JSON(http.StatusOK, m)
	})

	r.GET("/search-memories").Summary("Search all active memories by memory_id or content").QueryParam("q", "string").ResponseBody([]Memory{}).Handler(func(c *fuego.Context) error {
		q := c.Query("q")
		rows, err := db.Query(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE archived=0 AND (memory_id LIKE ? OR content LIKE ?) ORDER BY memory_id, version DESC`, "%"+q+"%", "%"+q+"%")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
		}
		defer rows.Close()
		var memories []Memory
		for rows.Next() {
			var m Memory
			var archivedBool bool
			if err := rows.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return c.JSON(http.StatusInternalServerError, fuego.H{"error": err.Error()})
			}
			m.Archived = archivedBool
			memories = append(memories, m)
		}
		return c.JSON(http.StatusOK, memories)
	})

	r.Run(":8080")
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
