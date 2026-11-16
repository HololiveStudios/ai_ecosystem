package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SavedMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

var protectedSystemFiles = map[string]bool{
	"main.go":            true,
	"telemetry.go":       true,
	"llama_engine.go":    true,
	"context_manager.go": true,
	"system_tools.go":    true,
	"system_stats.go":    true,
	"index.html":         true,
	"go.mod":             true,
	"go.sum":             true,
	"gateway_cgo":        true,
	"rag.go":             true,
}

var binaryToPkgMap = map[string]string{
	"npm":     "nodejs",
	"node":    "nodejs",
	"python":  "python",
	"python3": "python",
	"pip":     "python",
	"unzip":   "unzip",
	"tar":     "tar",
	"git":     "git",
	"curl":    "curl",
	"wget":    "wget",
	"jq":      "jq",
	"make":    "make",
	"clang":   "clang",
	"7z":      "p7zip",
}

func GetProjectsRootDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/data/data/com.termux/files/home"
	}
	return filepath.Join(homeDir, "projects")
}

func ValidateAndResolvePath(projectID string, targetPath string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", fmt.Errorf("PROJETO NÃO DEFINIDO: Informe o projeto ativo.")
	}

	baseName := filepath.Base(targetPath)
	if protectedSystemFiles[baseName] {
		return "", fmt.Errorf("AÇÃO BLOQUEADA: '%s' faz parte do ecossistema do Gateway protegido.", baseName)
	}

	projectDir := filepath.Join(GetProjectsRootDir(), projectID)
	fullPath := filepath.Join(projectDir, targetPath)

	cleanPath := filepath.Clean(fullPath)
	cleanProjectDir := filepath.Clean(projectDir)

	if !strings.HasPrefix(cleanPath, cleanProjectDir) {
		return "", fmt.Errorf("AÇÃO BLOQUEADA: Fora do diretório do projeto '%s'.", projectID)
	}

	return cleanPath, nil
}

// Executa comandos e ativa o Self-Healing se ocorrer algum erro de compilação ou runtime
func ExecuteProjectCommand(projectID string, commandStr string) (string, error) {
	if strings.TrimSpace(projectID) == "" {
		return "", fmt.Errorf("Nenhum projeto selecionado.")
	}

	projectDir := filepath.Join(GetProjectsRootDir(), projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", fmt.Errorf("falha ao criar pasta do projeto: %w", err)
	}

	output, err := runShell(projectDir, commandStr)
	if err == nil {
		return output, nil
	}

	// 1. Auto-instalação de binários ausentes (Self-Healing de ambiente)
	missingBinary := extractMissingBinary(output)
	if missingBinary != "" {
		pkgName := missingBinary
		if mapped, exists := binaryToPkgMap[missingBinary]; exists {
			pkgName = mapped
		}

		installCmd := fmt.Sprintf("pkg install -y %s", pkgName)
		_, installErr := runShell(projectDir, installCmd)
		if installErr == nil {
			retryOutput, retryErr := runShell(projectDir, commandStr)
			if retryErr == nil {
				return fmt.Sprintf("[Auto-Install: %s instalado com sucesso]\n%s", pkgName, retryOutput), nil
			}
		}
	}

	// 2. Self-Healing de código (compilação/execução mal sucedida)
	isCodeErr := strings.Contains(commandStr, "gcc") || 
		strings.Contains(commandStr, "clang") || 
		strings.Contains(commandStr, "go run") || 
		strings.Contains(commandStr, "python") || 
		strings.Contains(commandStr, "node") || 
		strings.HasPrefix(commandStr, "./")

	if isCodeErr {
		healedOutput, healingErr := HandleSelfHealing(projectID, commandStr, output)
		if healingErr == nil {
			return healedOutput, nil
		}
	}

	return output, err
}

func runShell(dir string, cmdStr string) (string, error) {
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = dir

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	combined := strings.TrimSpace(out.String() + "\n" + stderr.String())
	return combined, err
}

// Orquestrador de diagnóstico e correção automática
func HandleSelfHealing(projectID, commandStr, errorLog string) (string, error) {
	var targetFile string
	lines := strings.Split(errorLog, "\n")
	
	// Identifica qual arquivo gerou o erro de compilação/runtime
	for _, line := range lines {
		if strings.Contains(line, ".c:") || strings.Contains(line, ".cpp:") || strings.Contains(line, ".go:") || strings.Contains(line, ".py:") {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				targetFile = filepath.Base(strings.TrimSpace(parts[0]))
				break
			}
		}
	}

	if targetFile == "" {
		return "", fmt.Errorf("arquivo com erro não identificado no log")
	}

	originalCodeBytes, err := ReadProjectFile(projectID, targetFile)
	if err != nil {
		return "", err
	}
	originalCode := string(originalCodeBytes)

	// Solicita correção ao Llama 3.2 1B (Executor)
	healingPrompt := fmt.Sprintf(
		"Você é o assistente técnico de Self-Healing. O seguinte arquivo apresentou erro.\n"+
		"Arquivo: %s\n\n"+
		"Código Original:\n```\n%s\n```\n\n"+
		"Erro retornado:\n```\n%s\n```\n\n"+
		"Responda APENAS o código completo corrigido usando a sintaxe markdown ``` correspondente. Não dê explicações extras.",
		targetFile, originalCode, errorLog,
	)

	fmt.Printf("[SELF-HEALING] Identificado erro em %s. Solicitando correção...\n", targetFile)
	correctedResponse, err := ProcessLLaMARequest(projectID, healingPrompt)
	if err != nil {
		return "", err
	}

	ext := filepath.Ext(targetFile)
	lang := strings.TrimPrefix(ext, ".")
	correctedCode := extractCodeBlock(correctedResponse, lang)

	if correctedCode == "" {
		return "", fmt.Errorf("não foi possível extrair o código corrigido da resposta")
	}

	// Sobrescreve o arquivo no workspace do projeto
	err = WriteProjectFile(projectID, targetFile, correctedCode)
	if err != nil {
		return "", err
	}

	// Adiciona a correção ao histórico de longo prazo para o sistema aprender
	_ = AddToLongTermMemory(projectID, fmt.Sprintf("Erro resolvido no arquivo %s:\nCódigo corrigido:\n%s", targetFile, correctedCode))

	// Reexecuta o comando original
	retryOutput, retryErr := runShell(filepath.Join(GetProjectsRootDir(), projectID), commandStr)

	statusLabel := "✅ **[SELF-HEALING BEM SUCEDIDO]**"
	if retryErr != nil {
		statusLabel = "⚠️ **[SELF-HEALING TENTADO - ERRO PERSISTENTE]**"
	}

	resultMsg := fmt.Sprintf(
		"%s\n\n"+
		"❌ **Erro Original no arquivo `%s`**:\n```\n%s\n```\n"+
		"🔧 **Correção efetuada automaticamente no arquivo!**\n\n"+
		"🚀 **Resultado da nova execução**:\n%s",
		statusLabel, targetFile, errorLog, retryOutput,
	)

	return resultMsg, nil
}

func extractMissingBinary(errOutput string) string {
	lines := strings.Split(errOutput, "\n")
	for _, line := range lines {
		if strings.Contains(line, "command not found") || strings.Contains(line, "No such file or directory") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				bin := strings.Trim(parts[0], "':\"")
				bin = filepath.Base(bin)
				if bin != "" && bin != "bash" {
					return bin
				}
			}
		}
	}
	return ""
}

func WriteProjectFile(projectID string, relPath string, content string) error {
	finalPath, err := ValidateAndResolvePath(projectID, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return fmt.Errorf("falha ao criar subdiretórios: %w", err)
	}
	return os.WriteFile(finalPath, []byte(content), 0644)
}

func ReadProjectFile(projectID string, relPath string) ([]byte, error) {
	finalPath, err := ValidateAndResolvePath(projectID, relPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(finalPath)
}

func LoadProjectHistory(projectID string) ([]SavedMessage, error) {
	bytes, err := ReadProjectFile(projectID, ".chat_history.json")
	if err != nil {
		return nil, err
	}
	var history []SavedMessage
	err = json.Unmarshal(bytes, &history)
	return history, err
}

func SaveProjectHistory(projectID string, history []SavedMessage) error {
	bytes, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return WriteProjectFile(projectID, ".chat_history.json", string(bytes))
}
