# Tarefa 3.0: Truncamento de tool results gigantes como garantia dura

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Truncamento de tool results gigantes

## Dependências

- 2.0 (compação implementada; truncamento roda pós-compação quando ainda estoura)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

O assoalho que torna "zero estouros" verdadeiro: a compação depende de LLM (pode falhar), o truncamento é mecânico e sempre cabe. Pós-compação, se a re-estimativa ainda estourar, `truncateOldToolResults()` percorre `messages` da mais antiga para a mais recente; mensagens `role: "tool"` com conteúdo > 2000 chars são truncadas para `Content[:2000] + "… (truncated)"`; a estimativa é refeita após cada truncamento; o loop para quando a estimativa cabe ou não truncou nada. A cauda preservada (últimas 4) nunca é truncada. Este caminho também cobre a falha da chamada de resumo da task 2.0 — a garantia de não-estouro nunca depende do LLM de resumo funcionar.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib (`strings`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código.
</skills>

<requirements>
- Constante `toolResultTruncateChars = 2000`.
- `truncateOldToolResults() bool` — retorna se truncou algo; percorre da mais antiga para a mais recente.
- Mensagens `role: "tool"` com `len(Content) > 2000` → `Content[:2000] + "… (truncated)"`.
- Re-estimar após cada truncamento; parar quando a estimativa cabe ou nada foi truncado.
- Últimas 4 mensagens (cauda preservada) nunca truncadas.
- Falha da chamada de resumo → fluxo cai no truncamento; turno completa sem `EventError`.
- Ordem no loop: `decide` → `compact` → re-estimar → `truncate` (loop até caber) → `ChatStream`.
</requirements>

## Subtarefas

- [ ] 3.1 Implementar `truncateOldToolResults` com limite de 2000 chars + sufixo
- [ ] 3.2 Inserir o loop pós-compação (re-estimar → truncar → repetir) no fluxo antes do `ChatStream`
- [ ] 3.3 Garantir que a falha de resumo da task 2.0 degrada para o truncamento
- [ ] 3.4 Escrever os testes 6-8 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Regras de `truncateOldToolResults` e ordem no loop: techspec, seção **Design de Implementação** (subseção "Regras") e **Verificações Técnicas → Arquitetura**.
- Decisão "truncamento como garantia dura": techspec, seção **Considerações Técnicas → Decisões Principais** (item 3).
- Invariante da cauda: techspec, seção **Verificações Técnicas → Arquitetura**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: tool result de 50k chars numa janela de 2k é truncado para 2000 + sufixo; a chamada seguinte não estoura.
- Teste prova: tool result gigante dentro das últimas 4 mensagens não é truncado.
- Teste prova: mock falhando a chamada de resumo → truncamento executa e o turno completa sem `EventError`.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, conforme techspec **Abordagem de Testes**):
  - `TestTruncationRescuesGiantToolResult` — tool result de 50k chars em janela de 2k: truncado para 2000 + sufixo; chamada seguinte não estoura.
  - `TestTruncationPreservesTail` — tool result gigante nas últimas 4 mensagens não é truncado.
  - `TestSummaryFailureFallsBackToTruncation` — mock falha o resumo → truncamento executa; turno completa sem `EventError`.
- [ ] Testes de integração — cobertos pelos testes de agent com mock gateway.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `truncateOldToolResults`, loop pós-compação
- `internal/agent/agent_test.go` — testes de truncamento e fallback
