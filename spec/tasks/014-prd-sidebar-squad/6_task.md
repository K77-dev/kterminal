# Tarefa 6.0: Testes de integração E2E, regressão completa e dogfooding documentado

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Exibição condicionada ao modo squad (toggle de modo no fluxo completo)
- REQ-002 — Painel da mesa (mesa reconstruída no resume)
- REQ-003 — Atividade ao vivo (fluxo com tools no mock gateway)
- REQ-004 — Métricas por persona e orçamento da mesa (sidebar × transcript JSONL)
- REQ-005 — Layout responsivo (resize ao vivo no dogfooding)
- REQ-006 — Persistência e resume (resume `--continue` e snapshot pré-feature)

## Dependências

- 5.0 (feature completa integrada)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Fechar o ciclo: testes de integração ponta-a-ponta (resume e fluxo completo com mock gateway), regressão completa da suíte e dogfooding documentado — a mesa real rodando no próprio kterminal, conferindo o sidebar contra o transcript JSONL, com resize ao vivo e interrupção com Esc. Sem TestSprite (TUI local — precedente da feature 012).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): verificação completa `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`; dogfooding documentado como E2E (precedente feature 012).
</skills>

<requirements>
- Regressão completa verde: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
- Testes de integração da techspec: resume e fluxo completo com mock gateway
- Dogfooding documentado (relatório curto na pasta da feature ou no corpo do PR): mesa real, resize 80/120/200, Esc mid-convocation, resume, conferência sidebar × JSONL
</requirements>

## Subtarefas

- [ ] 6.1 Teste de integração de resume: sessão em squad com mesa encerrada → `--continue` reconstrói personas/status/métricas no painel; snapshot pré-feature carrega e exibe "mesa não iniciada"
- [ ] 6.2 Teste de integração de fluxo completo com mock gateway: kickoff → convocações → convergência → `/mode sdd` (sidebar some) → `/mode squad` (mesa preservada) → novo kickoff reconstrói a Mesa
- [ ] 6.3 Regressão completa: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
- [ ] 6.4 Dogfooding documentado: mesa real no kterminal com resize ao vivo (80/120/200 cols), interrupção com Esc e resume, conferindo valores do sidebar contra o transcript JSONL

## Detalhes de Implementação

Consulte techspec.md — seção "Abordagem de Testes" (testes de integração e E2E). Os testes de integração usam os mocks existentes de `agent_test.go`/gateway; o dogfooding é manual e documentado — capturar evidências (descrição do cenário, valores conferidos) no relatório.

## Critérios de Sucesso

- Resume reconstrói a mesa com valores idênticos aos persistidos; snapshot pré-feature exibe "mesa não iniciada" sem erro
- Fluxo completo com mock gateway transita por todos os estados sem vazamento entre modos (mesa preservada no toggle, reconstruída no novo kickoff)
- Regressão completa verde, incluindo `-race`
- Dogfooding: valores de tokens/custo do sidebar batem com o transcript JSONL; layout íntegro de 80 a 200+ cols; Esc marca persona `done` e o painel reflete o estado final

## Testes da Tarefa

- [ ] Testes de unidade: N/A (cobertos nas tasks 1–5)
- [ ] Testes de integração: subtarefas 6.1 e 6.2
- [ ] Testes E2E: dogfooding documentado (subtarefa 6.4)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent_test.go` (modificado — testes de integração de resume/fluxo)
- `internal/session/session_test.go` (modificado — snapshot pré-feature no resume)
- `internal/tui/tui_test.go` (modificado — fluxo completo Agent→TUI, se necessário)
- `spec/tasks/014-prd-sidebar-squad/` (relatório de dogfooding)
