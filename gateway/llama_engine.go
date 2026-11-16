package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func ProcessLLaMARequest(projectID string, prompt string) (string, error) {
	url := "http://127.0.0.1:8080/v1/chat/completions"

	history, err := LoadProjectHistory(projectID)
	var messages []map[string]string

	messages = append(messages, map[string]string{
		"role":    "system",
		"content": "Você é um assistente de IA autônomo executando localmente em um ecossistema inteligente. Seja direto e objetivo.",
	})

	if err == nil {
		// Pega as últimas 6 mensagens para manter o contexto sem estourar os limites
		startIdx := 0
		if len(history) > 6 {
			startIdx = len(history) - 6
		}
		for _, h := range history[startIdx:] {
			messages = append(messages, map[string]string{
				"role":    h.Role,
				"content": h.Content,
			})
		}
	}

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": prompt,
	})

	payload := map[string]interface{}{
		"model":       "local-model",
		"messages":    messages,
		"temperature": 0.3,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llama-server inativo ou inacessível na porta 8080: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("resposta vazia do modelo local")
}

func extractCodeBlock(response string, lang string) string {
	startMarker := "```" + lang
	startIdx := strings.Index(response, startMarker)
	if startIdx == -1 {
		startMarker = "```"
		startIdx = strings.Index(response, startMarker)
		if startIdx == -1 {
			return response // Se não houver blocos, retorna tudo
		}
	}

	startIdx += len(startMarker)
	endIdx := strings.Index(response[startIdx:], "```")
	if endIdx == -1 {
		return strings.TrimSpace(response[startIdx:])
	}

	return strings.TrimSpace(response[startIdx : startIdx+endIdx])
}
