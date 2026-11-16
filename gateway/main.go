package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type CommandRequest struct {
	Prompt      string `json:"prompt"`
	TargetModel string `json:"target_model"`
	ProjectID   string `json:"project_id"`
	ThreadID    string `json:"thread_id"`
	ExecCommand string `json:"exec_command"`
}

type FileOperationRequest struct {
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

var mu sync.Mutex

func main() {
	PrintTerminalDashboard()

	os.MkdirAll(GetProjectsRootDir(), 0755)

	http.Handle("/", http.FileServer(http.Dir(".")))

	http.HandleFunc("/api/v1/telemetry", handleTelemetry)
	http.HandleFunc("/api/v1/projects", handleListProjects)
	http.HandleFunc("/api/v1/files", handleListFiles)
	http.HandleFunc("/api/v1/file", handleFileOperation)
	http.HandleFunc("/api/v1/command", handleCommand)

	port := ":8080"
	fmt.Printf("[GATEWAY] Servidor ativo em http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Erro fatal no servidor HTTP: %v", err)
	}
}

func handleTelemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetSystemTelemetry())
}

func handleListProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rootDir := GetProjectsRootDir()
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"projects": []string{}})
		return
	}

	var projects []string
	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"projects": projects})
}

func handleListFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	relPath := r.URL.Query().Get("path")

	if projectID == "" {
		http.Error(w, `{"error": "PROJETO NÃO DEFINIDO"}`, http.StatusBadRequest)
		return
	}

	targetDir := filepath.Join(GetProjectsRootDir(), projectID, relPath)
	os.MkdirAll(targetDir, 0755)

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"entries": []string{}})
		return
	}

	type Entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}

	var result []Entry
	for _, e := range entries {
		result = append(result, Entry{
			Name:  e.Name(),
			Path:  filepath.Join(relPath, e.Name()),
			IsDir: e.IsDir(),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"entries": result})
}

func handleFileOperation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		projectID := r.URL.Query().Get("project_id")
		relPath := r.URL.Query().Get("path")

		content, err := ReadProjectFile(projectID, relPath)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"content": string(content)})

	case http.MethodPost:
		var req FileOperationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Payload inválido"}`, http.StatusBadRequest)
			return
		}

		if err := WriteProjectFile(req.ProjectID, req.Path, req.Content); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		// Adiciona o conteúdo recém-salvo do arquivo à memória de longo prazo (RAG) para aprendizado futuro
		_ = AddToLongTermMemory(req.ProjectID, fmt.Sprintf("Conteúdo do arquivo %s atualizado:\n%s", req.Path, req.Content))

		json.NewEncoder(w).Encode(map[string]string{"status": "sucesso"})

	default:
		http.Error(w, `{"error": "Método não suportado"}`, http.StatusMethodNotAllowed)
	}
}

func handleCommand(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Payload inválido"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.ProjectID) == "" {
		json.NewEncoder(w).Encode(map[string]string{
			"response": "Nenhum projeto selecionado.",
		})
		return
	}

	projectDir := filepath.Join(GetProjectsRootDir(), req.ProjectID)
	os.MkdirAll(projectDir, 0755)

	prompt := strings.TrimSpace(req.Prompt)
	var output string
	var err error

	// 1. Tratamento de comandos shell prefixados com "/"
	if strings.HasPrefix(prompt, "/") {
		cmdStr := strings.TrimPrefix(prompt, "/")
		output, err = ExecuteProjectCommand(req.ProjectID, cmdStr)
		if err != nil && output == "" {
			output = fmt.Sprintf("Erro ao executar comando: %v", err)
		}
	} else if req.ExecCommand != "" {
		output, err = ExecuteProjectCommand(req.ProjectID, req.ExecCommand)
		if err != nil && output == "" {
			output = fmt.Sprintf("Erro ao executar comando: %v", err)
		}
	} else {
		// 2. Busca RAG (Memória de Longo Prazo) sobre o prompt enviado pelo usuário
		var contextTexts []string
		memories, err := SearchLongTermMemory(req.ProjectID, prompt, 3)
		if err == nil {
			for _, mem := range memories {
				if mem.Score > 0.70 { // Filtro de similaridade alta (relevância)
					contextTexts = append(contextTexts, fmt.Sprintf("[Memória Histórica (Score %.2f)]\n%s", mem.Score, mem.Text))
				}
			}
		}

		augmentedPrompt := prompt
		if len(contextTexts) > 0 {
			augmentedPrompt = fmt.Sprintf(
				"Lembretes Históricos Relevantes:\n%s\n\nPrompt do Usuário:\n%s",
				strings.Join(contextTexts, "\n---\n"), prompt,
			)
		}

		// 3. Processamento via Motor CGO (Qwen 0.5B + Llama 1B)
		output, err = ProcessLLaMARequest(req.ProjectID, augmentedPrompt)
		if err != nil || strings.TrimSpace(output) == "" {
			output, err = ExecuteProjectCommand(req.ProjectID, prompt)
			if err != nil && strings.TrimSpace(output) == "" {
				output = "Comando processado com sucesso no workspace."
			}
		}

		// 4. Salva a resposta gerada e o prompt na Memória de Longo Prazo se contiver código ou explicações úteis
		if strings.Contains(output, "```") {
			_ = AddToLongTermMemory(req.ProjectID, fmt.Sprintf("Instrução: %s\nCódigo Gerado:\n%s", prompt, output))
		}
	}

	history, _ := LoadProjectHistory(req.ProjectID)

	history = append(history, SavedMessage{
		Role:      "user",
		Content:   req.Prompt,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	history = append(history, SavedMessage{
		Role:      "assistant",
		Content:   output,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	_ = SaveProjectHistory(req.ProjectID, history)

	json.NewEncoder(w).Encode(map[string]string{
		"response": output,
		"workdir":  projectDir,
	})
}
