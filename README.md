# AI Ecosystem - Gateway & RAG Engine

## 1. Objetivo e Finalidade
Gateway central de inteligência artificial com engine LLaMA integrada, sistema RAG (Retrieval-Augmented Generation) e monitoramento de telemetria.

## 2. Tecnologias Utilizadas
- Go (Golang), HTML5, Local LLM Engine, System Telemetry

## 3. Arquitetura e Estrutura de Diretórios
Serviço monolítico em Go estruturado na pasta `gateway/`, expondo endpoints HTTP para inferência de modelos locais, processamento vetorial RAG e métricas de hardware.

## 4. Principais Arquivos e Responsabilidades
- `gateway/main.go`: Ponto de entrada do servidor web e roteamento da API.
- `gateway/llama_engine.go`: Integração com mecanismo de inferência LLaMA.
- `gateway/rag.go`: Lógica de recuperação aumentada por geração (RAG).
- `gateway/system_stats.go`: Coletor de métricas de CPU, memória e GPU.
- `gateway/system_tools.go`: Ferramentas utilitárias do sistema operacional.
- `gateway/index.html`: Dashboard web integrado.

## 5. Funcionalidades Identificadas
- Execução e inferência com modelos locais de linguagem
- Busca contextual com pipeline RAG
- Telemetria de uso de recursos do computador
- Dashboard embutido para monitoramento e chat

## 6. Como Compilar ou Executar
```bash
cd gateway
go run main.go llama_engine.go rag.go system_stats.go system_tools.go
```

## 7. Estado Atual do Projeto
**Completo**

## 8. Pendências e Limitações Identificadas
Adicionar suporte a mais quantizações GGUF e modelos multimodais.
