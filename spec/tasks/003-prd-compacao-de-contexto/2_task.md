# Tarefa 2.0: Compação automática em 70% da janela com evento e transcript

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Compação automática em 70% da janela
- REQ-004 — Evento de compação

## Dependências

- 1.0 (`estimateTokens` disponível e testado)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Núcleo da feature: quando a estimativa excede 70% do `ContextWindow` do modelo decidido, a compação executa **antes** da chamada — os turnos antigos (tudo exceto as últimas 4 mensagens) vão a um modelo barato fixo (`deepseek-v4.1-flash`, fallback para o default do catálogo) via `ChatStream` sem tools, pedindo um resumo denso; o histórico é substituído por `[system message com o resumo] + últimas 4 mensagens`. Um novo `EventCompaction` (tokens antes/depois) é emitido ao canal e gravado no transcript como evento `compaction` — `session.Event` ganha `TokensBefore/TokensAfter` (`omitempty`). A compação é mecânica: limiar de tokens, zero interação com o usuário. Falha da chamada de resumo aborta a compação silenciosamente (o fluxo cai no truncamento da task 3.0).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (sessão longa E2E), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante).
</skills>

<requirements>
- Constantes: `compactionThreshold = 0.7`, `preservedTailMessages = 4`, `summaryModel = "deepseek-v4.1-flash"`.
- `needsCompaction(window int) bool` e `compact(ctx) error` como métodos privados do `Agent`.
- Ordem no loop: `decide` → `needsCompaction(window do modelo decidido)` → `compact` → `ChatStream` (compação após o roteamento — janela do modelo certo).
- `buildSummaryPrompt` serializa os turnos antigos (role + content truncado a 2000 chars por mensagem) pedindo resumo denso: decisões, arquivos tocados, estado da tarefa.
- Resumo entra como mensagem `role: "system"` (padrão OpenAI-compatible) + últimas 4 mensagens byte-idênticas.
- Chamada de resumo sem tools; modelo de resumo fixo (não roteado); fallback para `Catalog.DefaultModel` se ausente/indisponível.
- `EventCompaction` emitido com `TokensBefore > TokensAfter`; evento `compaction` gravado no transcript com `tokens_before`/`tokens_after`.
- `session.Event` ganha `TokensBefore/TokensAfter int64` com `omitempty` — eventos existentes permanecem byte-idênticos.
- Falha da chamada de resumo → compação aborta silenciosamente, sem `EventError` (a garantia de não-estouro é o truncamento da task 3.0).
- Últimas 4 mensagens nunca compactadas nem truncadas.
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `EventCompaction` ao `EventKind` e campos `TokensBefore/TokensAfter` ao `agent.Event` e `session.Event`
- [ ] 2.2 Implementar `needsCompaction` e `buildSummaryPrompt`
- [ ] 2.3 Implementar `compact` (chamada de resumo sem tools, system message, substituição do histórico, evento + transcript)
- [ ] 2.4 Inserir a checagem no loop após `decide` e antes do `ChatStream`
- [ ] 2.5 Escrever os testes 1-4 da techspec com catálogo de janela pequena e mock gateway (ver Testes da Tarefa)

## Detalhes de Implementação

- Snippet completo de `compact` e constantes: techspec, seção **Design de Implementação → Interfaces Principais**.
- Regras do prompt de resumo, fallback de modelo e falha silenciosa: techspec, seção **Design de Implementação** (subseção "Regras").
- Ordem no loop e invariante da cauda preservada: techspec, seção **Verificações Técnicas → Arquitetura**.
- Decisões "role system para o resumo", "compação após decide" e "resumo não-roteado": techspec, seção **Considerações Técnicas → Decisões Principais** (itens 1, 2, 4).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste mandatório prova: com catálogo de janela 2k, a chamada de resumo (modelo `deepseek-v4.1-flash`, sem tools no request) ocorre **antes** da chamada que estouraria; a sessão continua funcionando (`EventTurnDone`).
- Pós-compação, `messages` = `[system] + últimas 4 originais` byte-idênticas.
- Fallback de modelo e evento/transcript cobertos por teste.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, catálogo de teste com `ContextWindow: 2000` e mock gateway contando chamadas — techspec **Abordagem de Testes**):
  - `TestCompactionBeforeOverflow` (mandatório) — chamada de resumo antes da chamada que estouraria; sessão continua.
  - `TestCompactionPreservesLastFour` — `[system] + últimas 4` byte-idênticas.
  - `TestCompactionUsesFallbackModel` — `deepseek-v4.1-flash` ausente → default do catálogo.
  - `TestCompactionEmitsEventAndTranscript` — `EventCompaction` com `TokensBefore > TokensAfter`; evento `compaction` no transcript.
- [ ] Testes de integração — cobertos pelos testes de agent com mock gateway (fluxo decide→compact→call).
- [ ] Testes E2E — deferidos para `kspec-qa` (sessão longa real compacta e continua; coesão da tarefa pós-compação).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `needsCompaction`, `compact`, `buildSummaryPrompt`, `EventCompaction`, campos no `Event`
- `internal/agent/agent_test.go` — catálogo de janela pequena, contagem de chamadas do mock
- `internal/session/session.go` — campos `TokensBefore/TokensAfter` no `Event`
- `internal/llm/llm.go` — sem mudança (`ChatStream` sem tools reusado)
- `internal/catalog/catalog.go` — sem mudança (`ContextWindow`, `DefaultModel` consumidos)
