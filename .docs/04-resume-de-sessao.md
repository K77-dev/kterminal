# 04 — Retomar sessão (`kterminal --continue`)

## Contexto

kterminal é um terminal agêntico em Go. Toda sessão já é gravada como transcript JSONL em `~/.local/share/kterminal/sessions/` (`internal/session/session.go`), mas ao fechar o app a conversa se perde — não há resume (decisão deliberada do v0.1).

Arquitetura relevante:

- `internal/session/session.go` — `Writer` (JSONL append), `Event` com campos fixos; arquivos nomeados `20060102-150405.jsonl`.
- `internal/agent/agent.go` — `Agent.messages []llm.Message` é a fonte da verdade da conversa.
- `main.go` — flags `--confirm`, `--doctor`.

## Objetivo

`kterminal --continue` reabre a última sessão com o histórico completo restaurado, pronto para continuar a conversa. `kterminal --session <caminho>` abre uma sessão específica.

## Especificação

1. **Snapshot em vez de replay**: ao final de cada turno (`EventTurnDone` e também em abort/erro pós-stream), o agent grava no transcript um evento `{"type": "snapshot", "messages": [...]}` com o array `[]llm.Message` serializado inteiro (JSON). Isso evita reconstruir mensagens de tool calls a partir de eventos granulares — o snapshot é a fonte da verdade.
2. `session.Writer` ganha método `WriteSnapshot(messages []llm.Message)`; `session.Event` ganha campo `Messages json.RawMessage`.
3. Nova função `session.LoadLatest() (path string, messages []llm.Message, err error)` e `session.Load(path)`: lê o arquivo de trás para frente, pega o ÚLTIMO evento `snapshot`, decodifica as mensagens. Sem snapshot no arquivo → erro explícito ("sessão sem snapshot").
4. `main.go`: flag `--continue` e `--session <path>`. Com `--continue`: pega o arquivo mais recente em `session.Dir()` (ordenado por nome), carrega o snapshot, injeta em `agent.SetMessages(messages)`. Se não houver sessões: aviso no chat (bloco `colTextMuted`: "no previous session — starting fresh") e segue.
5. `Agent` ganha `SetMessages([]llm.Message)` (também zera `candidates` para revalidar contra o gateway).
6. A sessão retomada **continua no mesmo arquivo** de transcript (o `Writer` abre o arquivo existente em modo append) para manter o histórico completo num lugar só.
7. TUI: ao iniciar com sessão carregada, renderizar as últimas mensagens no viewport (reconstruir blocos a partir das mensagens: role user → userBox, role assistant → markdown + linha `▣`, role tool → linha de tool) e mostrar no hint bar `resumed · N mensagens`.

## Critérios de aceitação

- Fluxo: conversar, sair, `kterminal --continue` → histórico visível, pergunta nova responde com contexto anterior.
- Snapshot após cada turno; o resume usa o mais recente.
- `--session` com arquivo inexistente → erro claro no stderr, exit 1.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: roundtrip — criar Writer, simular 3 turnos com snapshots, `LoadLatest` retorna mensagens idênticas às originais (comparar JSON); snapshot parcial (arquivo com eventos mas sem snapshot) → erro.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Não reconstruir mensagens a partir dos eventos granulares — só snapshot.
