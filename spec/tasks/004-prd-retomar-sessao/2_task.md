# Tarefa 2.0: Gravação de snapshot nos fins de turno e `SetMessages` no Agent

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Snapshot por turno

## Dependências

- 1.0 (`WriteSnapshot` disponível em `internal/session`)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

O agent passa a gravar o snapshot nos três pontos de fim de turno — concluído (`EventTurnDone`), abortado (`EventTurnAborted`) e erro pós-stream — sempre com `a.messages` no estado corrente (o estado pós-abort com tool results sintéticos é válido e retomável). Além disso, o novo `SetMessages([]llm.Message)` substitui o histórico (ponto de entrada do resume) e zera `candidates` e `sessionCost` — `candidates = nil` força `ensureCandidates` a revalidar contra o gateway no próximo turno (modelos/candidatos podem ter mudado entre sessões — restrição do PRD).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante).
</skills>

<requirements>
- Snapshot gravado após `EventTurnDone`, `EventTurnAborted` e erro pós-stream — sempre com o array completo de mensagens do agente.
- `SetMessages(messages []llm.Message)` — substitui `messages`, zera `candidates` e `sessionCost`.
- `candidates = nil` → `ensureCandidates` refaz `ListModels` no próximo `Run` (revalidação contra o gateway).
- O snapshot reflete fielmente o estado do agente no fim do turno (fonte da verdade — REQ-001).
</requirements>

## Subtarefas

- [ ] 2.1 Implementar `SetMessages` (substitui histórico, zera `candidates` e `sessionCost`)
- [ ] 2.2 Gravar `WriteSnapshot(a.messages)` após `EventTurnDone`
- [ ] 2.3 Gravar snapshot após `EventTurnAborted` e em erro pós-stream
- [ ] 2.4 Escrever os testes 6-8 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Pontos de gravação e regras de `SetMessages`: techspec, seção **Design de Implementação → Interfaces Principais** e **Arquitetura do Sistema** (componente `internal/agent/agent.go`).
- Racional "snapshot também em abort/erro pós-stream": techspec, seção **Considerações Técnicas → Decisões Principais** (item 5).
- Compatibilidade com a compação (techspec 003): snapshots gravam `messages` pós-compação (system message incluída) — techspec, seção **Sequenciamento de Desenvolvimento → Dependências Técnicas**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: turno completo via mock → transcript contém evento `snapshot` com as mensagens do turno.
- Teste prova: turno abortado → snapshot gravado com estado consistente (tool results sintéticos incluídos).
- Teste prova: `SetMessages` → `ensureCandidates` refaz `ListModels` no próximo `Run` (mock conta chamadas).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 6-8):
  - `TestSnapshotWrittenAfterTurnDone` — transcript contém evento `snapshot` com as mensagens do turno.
  - `TestSnapshotWrittenAfterAbort` — snapshot com estado consistente pós-abort.
  - `TestSetMessagesClearsCandidates` — `ensureCandidates` refaz `ListModels` no próximo `Run`.
- [ ] Testes de integração — cobertos pelos testes de agent com mock (turno → snapshot ponta a ponta).
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — snapshot nos fins de turno, `SetMessages`
- `internal/agent/agent_test.go` — snapshot pós-turno/abort, revalidação de candidatos
