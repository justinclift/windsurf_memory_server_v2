package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

// Memory represents a memory record
// swagger:model
type Memory struct {
	ID        int       `json:"id"`
	MemoryID  string    `json:"memory_id"` // descriptive title
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

	// Initialize schema
	_, err = db.Exec(readSchema())
	if err != nil {
		panic(err)
	}

	r := gin.Default()

	// Index route
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Windsurf Memory Server: See /swagger/index.html for API docs.")
	})

	// Save memory
	r.POST("/save-memory", func(c *gin.Context) {
		var req struct {
			MemoryID string `json:"memory_id"`
			Content  string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("/save-memory: bad request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var version int
		err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM memories WHERE memory_id = ?", req.MemoryID).Scan(&version)
		if err != nil {
			log.Printf("/save-memory: version query failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version++
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO memories (memory_id, version, content, archived, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, req.MemoryID, version, req.Content, now, now)
		if err != nil {
			log.Printf("/save-memory: insert failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "saved", "memory_id": req.MemoryID, "version": version})
	})

	// Update memory
	r.POST("/update-memory", func(c *gin.Context) {
		var req struct {
			MemoryID string `json:"memory_id"`
			Content  string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("/update-memory: bad request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		_, err := db.Exec("UPDATE memories SET archived=1 WHERE memory_id=? AND archived=0", req.MemoryID)
		if err != nil {
			log.Printf("/update-memory: archive failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var version int
		err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM memories WHERE memory_id = ?", req.MemoryID).Scan(&version)
		if err != nil {
			log.Printf("/update-memory: version query failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version++
		now := time.Now().UTC()
		_, err = db.Exec(`INSERT INTO memories (memory_id, version, content, archived, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, req.MemoryID, version, req.Content, now, now)
		if err != nil {
			log.Printf("/update-memory: insert failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "updated", "memory_id": req.MemoryID, "version": version})
	})

	// Delete memory (archive all)
	r.POST("/delete-memory", func(c *gin.Context) {
		var req struct {
			MemoryID string `json:"memory_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("/delete-memory: bad request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		_, err := db.Exec("UPDATE memories SET archived=1 WHERE memory_id=?", req.MemoryID)
		if err != nil {
			log.Printf("/delete-memory: archive all failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "archived", "memory_id": req.MemoryID})
	})

	// List memories (latest, not archived)
	r.GET("/list-memories", func(c *gin.Context) {
		rows, err := db.Query(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE archived=0 ORDER BY memory_id, version DESC`)
		if err != nil {
			log.Printf("/list-memories: query failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "details": "query failed"})
			return
		}
		defer rows.Close()
		var memories []Memory
		for rows.Next() {
			var m Memory
			var archivedBool bool
			if err := rows.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
				log.Printf("/list-memories: scan failed: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "details": "scan failed"})
				return
			}
			m.Archived = archivedBool
			memories = append(memories, m)
		}
		if err := rows.Err(); err != nil {
			log.Printf("/list-memories: rows.Err: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "details": "rows.Err failed"})
			return
		}
		c.JSON(http.StatusOK, memories)
	})

	// Get memory by id (latest, not archived)
	r.GET("/get-memory-by-id/:memory_id", func(c *gin.Context) {
		memoryID := c.Param("memory_id")
		row := db.QueryRow(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE memory_id=? AND archived=0 ORDER BY version DESC LIMIT 1`, memoryID)
		var m Memory
		var archivedBool bool
		if err := row.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
			log.Printf("/get-memory-by-id: scan failed: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		m.Archived = archivedBool
		c.JSON(http.StatusOK, m)
	})

	// Search memories (active only)
	r.GET("/search-memories", func(c *gin.Context) {
		q := c.Query("q")
		rows, err := db.Query(`SELECT id, memory_id, version, content, archived, created_at, updated_at FROM memories WHERE archived=0 AND (memory_id LIKE ? OR content LIKE ?) ORDER BY memory_id, version DESC`, "%"+q+"%", "%"+q+"%")
		if err != nil {
			log.Printf("/search-memories: query failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		var memories []Memory
		for rows.Next() {
			var m Memory
			var archivedBool bool
			if err := rows.Scan(&m.ID, &m.MemoryID, &m.Version, &m.Content, &archivedBool, &m.CreatedAt, &m.UpdatedAt); err != nil {
				log.Printf("/search-memories: scan failed: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			m.Archived = archivedBool
			memories = append(memories, m)
		}
		c.JSON(http.StatusOK, memories)
	})

	r.Run(":8080")
}

// readSchema returns the schema SQL for initializing the DB
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
