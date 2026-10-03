# 03 — Gestão de janela de contexto (compação)

## Contexto

kterminal é um terminal agêntico em Go onde o Jev escolhe o LLM a cada chamada. O histórico inteiro (`Agent.messages []llm.Message`) é enviado em toda chamada e cresce sem limite. Os modelos do catálogo têm janelas diferentes (`internal/catalog/models.yaml`: deepseek 128k, GLM 200k). Hoje, uma sessão longa estoura a janela do modelo escolhido e a chamada erro — é a limitação conhecida mais grave.

Arquitetura relevante:

- `internal/agent/agent.go` — `loop()` envia `a.messages` a cada chamada; `StreamResult.Usage.PromptTokens` traz a contagem real de tokens da última chamada.
- `internal/catalog/catalog.go` — `Model.ContextWindow` por modelo.
- `internal/llm/llm.go` — `ChatStream` retorna `Usage`.

## Objetivo

Nunca estourar a janela: quando o contexto se aproxima do limite do modelo que o Jev escolheu, compactar automaticamente o histórico (resumo dos turnos antigos) e seguir o trabalho.

## Especificação

1. `Agent` rastreia `lastPromptTokens int64` (atualizado a cada `StreamResult`).
2. Antes de cada chamada no loop, estimar o custo do próximo prompt: `lastPromptTokens + delta aproximado desde a última chamada` (fallback: `len(mensagens concatenadas)/4` quando não há medição ainda).
3. Se a estimativa exceder **70%** do `ContextWindow` do modelo escolhido: executar compação antes de chamar:
   - Montar um prompt de resumo com os turnos antigos (tudo exceto as últimas 4 mensagens) e pedir a um modelo barato fixo (`deepseek-v4.1-flash`, com fallback para o default do catálogo se indisponível) um resumo denso: decisões, arquivos tocados, estado da tarefa.
   - Substituir `messages` por: `[mensagem de sistema/resumo] + últimas 4 mensagens`.
   - Emitir novo evento `EventCompaction` (tokens antes/depois) e gravar no transcript.
   - A chamada de resumo usa o mesmo `ChatStream` sem tools.
4. Se mesmo compactado o prompt não couber (ex.: um único tool result gigante), truncar tool results antigos para os primeiros 2000 caracteres com sufixo `… (truncated)`.
5. TUI: `EventCompaction` renderiza linha sutil `⚡ context compacted (12.4k → 3.1k tokens)` em `colTextMuted`.

## Critérios de aceitação

- Sessão simulada com catálogo de janela pequena (ex.: 2k tokens) compacta antes de estourar e continua funcionando.
- O resumo preserva as últimas 4 mensagens intactas.
- A contagem de tokens usa medição real (`Usage.PromptTokens`) após a primeira resposta.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Teste em `internal/agent/agent_test.go`: catálogo de teste com `ContextWindow` pequeno; mock do gateway conta as chamadas — a chamada de resumo (modelo `deepseek-v4.1-flash`) deve ocorrer antes da chamada que estouraria; assertar que `messages` encolheu e que o evento `EventCompaction` foi emitido.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- A compação é mecânica (limiar de tokens) — não perguntar ao Jev.
