# Tarefa 1.0: Tipo `squad.Mesa` — estado persistível da mesa com transições seguras para concorrência

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Painel da mesa (estados `waiting`/`deliberating`/`done` por persona)
- REQ-004 — Métricas por persona e orçamento da mesa (campos de tokens/custo/tetos)
- REQ-006 — Persistência e resume (struct serializável que virá a ir ao snapshot)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-6h

## Visão Geral

Criar o tipo `Mesa` em `internal/squad/mesa.go`: o estado da mesa do squad mode (roles do kickoff, tetos, contadores do turno, entradas por persona com status/modelo/tokens/custo). É a fundação da feature — nenhum outro pacote muda nesta task. A `Mesa` tem mutex interno (acessada por 3 goroutines no fluxo final: runLoop, drain de subagent, reader da TUI) e métodos de transição que o Agent chamará nos pontos existentes do loop.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês, `squad` é pacote leaf (não importa `agent` — params primitivos), testes com `-race`.
</skills>

<requirements>
- Structs e assinaturas exatamente como na seção "Interfaces Principais" da techspec.md (`MesaEntry`, `Mesa`, `Reset`, `ResetTurn`, `AddTokens`, `AddConvocation`, `StartDeliberation`, `FinishDeliberation`, `FinishTurn`, `ObservePersona`)
- Tags JSON com `omitempty` em todos os campos opcionais (snapshot aditivo, precedente: campos `Skill` e `Mode`)
- Todos os métodos seguros para concorrência (mutex interno; nenhum acesso a campos sem lock)
- Status como strings literais em inglês: `waiting` | `deliberating` | `done`
- `ObservePersona` soma cumulativos do subagente ao baseline capturado em `StartDeliberation` — a entrada acumula entre convocations da mesma persona
- `FinishTurn` converte `deliberating` → `done` (turno abortado reflete estado final)
</requirements>

## Subtarefas

- [x] 1.1 Criar `internal/squad/mesa.go` com `MesaEntry` e `Mesa` (campos e tags JSON da techspec)
- [x] 1.2 Implementar `Reset(k Kickoff, disciplines map[string]string)` — reconstrói a Mesa a partir do kickoff: roles, tetos (`MaxConvocations`, `TokenBudget`), entradas na ordem de convocação com status `waiting`
- [x] 1.3 Implementar `ResetTurn`, `AddTokens`, `AddConvocation` — contadores do turno que espelharão `turnTokens`/`turnConvocations` do Agent
- [x] 1.4 Implementar `StartDeliberation` (captura baseline de tokens/custo da convocation), `FinishDeliberation`, `FinishTurn`
- [x] 1.5 Implementar `ObservePersona(name, model string, tokens, cost)` — drain de métricas cumulativas do subagente somadas ao baseline
- [x] 1.6 Testes de unidade: ciclo completo da Mesa, round-trip JSON com `omitempty`, concorrência sob `-race`

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" e "Arquitetura do Sistema" (componente `internal/squad/mesa.go`). O struct `Kickoff` já existe no pacote `squad` (feature 012). Nada de espelhos no Agent nesta task — apenas o tipo e suas transições.

## Critérios de Sucesso

- `go test ./internal/squad/ -race` verde
- Ciclo completo funciona: `Reset` → `ResetTurn` → `StartDeliberation` → `ObservePersona` (2x, acumulando sobre baseline) → `FinishDeliberation` → `FinishTurn` com statuses e métricas finais corretos
- Round-trip JSON: Mesa populada serializa/desserializa idêntica; Mesa zerada serializa sem campos `omitempty`
- 100 goroutines mutando a Mesa sob `-race` sem data race

## Testes da Tarefa

- [x] Testes de unidade (`mesa_test.go`):
  - [x] `Reset` com kickoff de 3 roles e disciplinas: entradas na ordem, status `waiting`, tetos preenchidos
  - [x] `ResetTurn` zera `Convocations`/`Tokens` sem tocar em `Entries`
  - [x] `StartDeliberation`/`FinishDeliberation` transitam status; persona inexistente é no-op seguro
  - [x] `ObservePersona` com baseline: segunda convocation da mesma persona acumula sobre a primeira; modelo é atualizado
  - [x] `FinishTurn` marca `deliberating` → `done`
  - [x] Round-trip JSON com `omitempty` (populada e vazia)
  - [x] Acesso concorrente (goroutines mutando + lendo) sob `-race`
- [x] Testes de integração: N/A (tipo isolado; integração acontece na 3.0)
- [x] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/squad/mesa.go` (novo)
- `internal/squad/mesa_test.go` (novo)
- `internal/squad/squad.go` (referência — struct `Kickoff` existente)
