package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type MemoryChunk struct {
	Text      string    `json:"text"`
	Embedding []float32 `json:"embedding"`
	Timestamp time.Time `json:"timestamp"`
}

type VectorDB struct {
	Chunks []MemoryChunk `json:"chunks"`
	mu     sync.RWMutex
}

// Computa a similaridade de cosseno entre dois vetores na CPU do Termux
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// Obtém embeddings do llama-server ou gateway local
func GetEmbedding(text string) ([]float32, error) {
	url := "http://127.0.0.1:8080/embedding"
	payload := map[string]string{"content": text}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llama-server não respondeu na porta 8080: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func loadVectorDB(projectID string) (*VectorDB, error) {
	dbPath := filepath.Join(GetProjectsRootDir(), projectID, ".vector_memory.json")
	db := &VectorDB{Chunks: make([]MemoryChunk, 0)}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return db, nil
	}

	data, err := os.ReadFile(dbPath)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, db)
	return db, err
}

func saveVectorDB(projectID string, db *VectorDB) error {
	dbPath := filepath.Join(GetProjectsRootDir(), projectID, ".vector_memory.json")
	db.mu.RLock()
	defer db.mu.RUnlock()

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dbPath, data, 0644)
}

// Adiciona um novo fragmento à memória persistente
func AddToLongTermMemory(projectID, text string) error {
	vector, err := GetEmbedding(text)
	if err != nil {
		return err // Ignora se o modelo de embedding não estiver ativo
	}

	db, err := loadVectorDB(projectID)
	if err != nil {
		return err
	}

	db.mu.Lock()
	db.Chunks = append(db.Chunks, MemoryChunk{
		Text:      text,
		Embedding: vector,
		Timestamp: time.Now(),
	})
	db.mu.Unlock()

	return saveVectorDB(projectID, db)
}

// Retorna os fragmentos mais similares da memória de longo prazo (RAG)
type SearchResult struct {
	Text  string  `json:"text"`
	Score float32 `json:"score"`
}

func SearchLongTermMemory(projectID, query string, topK int) ([]SearchResult, error) {
	queryVector, err := GetEmbedding(query)
	if err != nil {
		return nil, err
	}

	db, err := loadVectorDB(projectID)
	if err != nil {
		return nil, err
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var results []SearchResult
	for _, chunk := range db.Chunks {
		score := CosineSimilarity(queryVector, chunk.Embedding)
		results = append(results, SearchResult{
			Text:  chunk.Text,
			Score: score,
		})
	}

	// Ordenação simples de scores mais altos para os mais baixos
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}
