package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
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

const baseURL = "http://localhost:8080"

func postJSON(t *testing.T, path string, body interface{}) *http.Response {
	data, _ := json.Marshal(body)
	r, err := http.Post(baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	return r
}

func getJSON(t *testing.T, path string) *http.Response {
	r, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	return r
}

func startTestServer() (*exec.Cmd, error) {
	cmd := exec.Command("go", "run", "../backend/main.go")
	cmd.Env = append(os.Environ(), "MEMORY_SERVER_DSN=:memory:")

	logFile, err := os.Create("test_server.log")
	if err != nil {
		return nil, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Wait for server to be ready (basic polling)
	for i := 0; i < 20; i++ {
		r, err := http.Get(baseURL + "/")
		if err == nil && r.StatusCode == 200 {
			return cmd, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	cmd.Process.Kill()
	// Dump server log if startup failed
	logFile.Seek(0, 0)
	logContent, _ := ioutil.ReadAll(logFile)
	return nil, fmt.Errorf("server did not start in time. Backend log:\n%s", string(logContent))
}

func stopTestServer(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
}

func TestMemoryAPI(t *testing.T) {
	cmd, err := startTestServer()
	if err != nil {
		t.Fatalf("could not start test server: %v", err)
	}
	defer func() {
		// Attempt graceful shutdown via /shutdown endpoint
		http.Post(baseURL+"/shutdown", "application/json", nil)
		stopTestServer(cmd)
	}()

	memID := "test-memory-title"
	content1 := "This is the first version."
	content2 := "This is the updated version."

	// Clean slate: ensure delete
	postJSON(t, "/delete-memory", map[string]string{"memory_id": memID})

	// Save memory
	resp := postJSON(t, "/save-memory", map[string]string{"memory_id": memID, "content": content1})
	if resp.StatusCode != 200 {
		t.Fatalf("save-memory failed: %v", resp.Status)
	}

	// List memories
	resp = getJSON(t, "/list-memories")
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list-memories failed: %v\nBody: %s", resp.Status, string(body))
	}
	body, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	var memories []Memory
	if err := json.Unmarshal(body, &memories); err != nil {
		t.Fatalf("list-memories unmarshal: %v", err)
	}
	found := false
	for _, m := range memories {
		if m.MemoryID == memID && m.Content == content1 && !m.Archived {
			found = true
		}
	}
	if !found {
		t.Error("saved memory not found in list-memories")
	}

	// Update memory
	resp = postJSON(t, "/update-memory", map[string]string{"memory_id": memID, "content": content2})
	if resp.StatusCode != 200 {
		t.Fatalf("update-memory failed: %v", resp.Status)
	}

	// Get memory by id
	resp = getJSON(t, "/get-memory-by-id/"+memID)
	if resp.StatusCode != 200 {
		t.Fatalf("get-memory-by-id failed: %v", resp.Status)
	}
	body, _ = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	var m Memory
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("get-memory-by-id unmarshal: %v", err)
	}
	if m.Content != content2 || m.MemoryID != memID || m.Archived {
		t.Errorf("get-memory-by-id did not return latest active version")
	}

	// Search memories
	resp = getJSON(t, "/search-memories?q=updated")
	if resp.StatusCode != 200 {
		t.Fatalf("search-memories failed: %v", resp.Status)
	}
	body, _ = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte(content2)) {
		t.Error("search-memories did not find updated content")
	}

	// Delete memory (archive all)
	resp = postJSON(t, "/delete-memory", map[string]string{"memory_id": memID})
	if resp.StatusCode != 200 {
		t.Fatalf("delete-memory failed: %v", resp.Status)
	}

	// List memories (should not include deleted)
	resp = getJSON(t, "/list-memories")
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list-memories after delete failed: %v\nBody: %s", resp.Status, string(body))
	}
	body, _ = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if bytes.Contains(body, []byte(memID)) {
		t.Error("deleted memory still present in list-memories")
	}

	// --- Extended test: multiple memories, versions, and archiving ---
	mems := []struct{
		ID string
		Versions []string
	}{
		{"memA", []string{"A1", "A2", "A3"}},
		{"memB", []string{"B1", "B2"}},
		{"memC", []string{"C1"}},
	}
	// Clean slate
	for _, m := range mems {
		postJSON(t, "/delete-memory", map[string]string{"memory_id": m.ID})
	}
	// Insert all versions
	for _, m := range mems {
		for _, v := range m.Versions {
			resp := postJSON(t, "/save-memory", map[string]string{"memory_id": m.ID, "content": v})
			if resp.StatusCode != 200 {
				t.Fatalf("save-memory failed for %s: %v", m.ID, resp.Status)
			}
		}
	}
	// Archive memB (delete)
	resp = postJSON(t, "/delete-memory", map[string]string{"memory_id": "memB"})
	if resp.StatusCode != 200 {
		t.Fatalf("delete-memory failed for memB: %v", resp.Status)
	}
	// List memories and verify only latest, non-archived
	resp = getJSON(t, "/list-memories")
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list-memories failed: %v\nBody: %s", resp.Status, string(body))
	}
	body, _ = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	var listed []Memory
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("list-memories unmarshal: %v", err)
	}
	foundA, foundB, foundC := false, false, false
	for _, m := range listed {
		if m.MemoryID == "memA" && m.Content == "A3" && !m.Archived {
			foundA = true
		}
		if m.MemoryID == "memB" {
			foundB = true // should NOT be found (archived)
		}
		if m.MemoryID == "memC" && m.Content == "C1" && !m.Archived {
			foundC = true
		}
	}
	if !foundA || foundB != false || !foundC {
		t.Errorf("list-memories did not return expected latest non-archived: foundA=%v foundB=%v foundC=%v", foundA, foundB, foundC)
	}
}
