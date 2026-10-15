# Tarefa 7.0: Verificação final — suite completa e métricas do PRD via transcript JSONL

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Verificação das métricas: zero chamadas ao endpoint Typesafe com `enabled = false`; persistência entre restarts
- REQ-002 — Verificação das métricas: modelo fixo idêntico em 100% das rotas do transcript; nenhuma decisão por keyword
- REQ-003 — Verificação: escolha de modelo intacta após restart
- REQ-004 — Verificação: descoberta completa (`--doctor`, `/config`, status bar, `/help`, sugestões)

## Dependências

- 1.0, 2.0, 3.0, 4.0, 5.0, 6.0

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação completa do repo, adicionar o teste de integração ponta-a-ponta que materializa as métricas de sucesso do PRD (turno completo com tool loop e subagente, Jev desabilitado, mock httptest do Jev como testemunha de zero chamadas, transcript JSONL com 100% das rotas no modelo fixo) e conferir a retro-compatibilidade com config legada de ponta a ponta. Não introduz comportamento novo — apenas testes remanescentes e checagem das métricas.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): testes seguem o padrão `go test` + httptest existente (`agent_test.go`); sem TestSprite (TUI Go sem frontend web — seção "Testes E2E" da techspec).
</skills>

<requirements>
- Verificação completa verde: `go build ./... && go vet ./... && gofmt -l . && go test ./...`
- Teste de integração de turno completo (padrão httptest de `agent_test.go`): com Jev desabilitado, servidor mock do Jev não recebe nenhuma requisição em turno completo com tool loop
- Subagentes (`RunSyncPersona`) roteiam para o modelo fixo com Jev desabilitado; `Decision.Router == "fixed"`/`"pin"` conforme o caminho
- Inspeção do transcript JSONL da sessão de teste: todas as entradas de rota com o mesmo `Model` e `Router ∈ {pin, fixed}` com Jev desabilitado
- Persistência entre restarts: re-`Load` da config após `/jev off` e `/model <name>` preserva os valores
- Fixture de config legada (sem `enabled`/`model`) preserva comportamento atual de ponta a ponta
</requirements>

## Subtarefas

- [x] 7.1 Rodar e zelar pela verificação completa: `go build ./... && go vet ./... && gofmt -l . && go test ./...`
- [x] 7.2 Teste de integração em `agent_test.go`: turno completo com tool loop e subagente, Jev desabilitado — mock do Jev com zero requisições; transcript com 100% das rotas no modelo fixo (`Router ∈ {pin, fixed}`)
- [x] 7.3 Checagem de persistência e retro-compatibilidade: re-`Load` após `/jev off` + `/model <name>`; fixture legada de ponta a ponta

## Detalhes de Implementação

Consulte techspec.md — seções "Abordagem de Testes" (testes de integração e E2E via transcript) e "Métricas de Negócio" (as três métricas verificáveis do PRD). Precedentes de mock: `mockJev`/`newTestAgent` em agent_test.go, `gatewayModelsHandler`/`captureGateway` em tui_test.go.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` verde (gofmt sem arquivos listados)
- Com `enabled = false`: nenhuma requisição ao endpoint Typesafe em toda a sessão de teste, incluindo subagentes — métrica do PRD
- 100% das entradas de rota do transcript com o mesmo `Model` e `Router ∈ {pin, fixed}` — métrica do PRD
- Escolha de modelo intacta após restart (re-`Load`) — métrica do PRD
- Nenhuma decisão por keyword (`Router != "heuristic"` como primary) com Jev desabilitado

## Testes da Tarefa

- [ ] Testes de unidade: N/A (cobertura das tasks 1.0–6.0; aqui apenas confere suite verde)
- [x] Testes de integração:
  - [x] `agent_test.go`: turno completo com tool loop, Jev desabilitado, mock do Jev recebe zero requisições
  - [x] `agent_test.go`: subagente via `RunSyncPersona` roteia para o modelo fixo com `Decision.Router` esperado
  - [x] Transcript JSONL da sessão de teste: todas as rotas com mesmo `Model` e `Router ∈ {pin, fixed}`
  - [x] Re-`Load` da config após `/jev off` + `/model <name>` preserva `enabled = false` e `model`
  - [x] Fixture de config legada → comportamento atual preservado (Jev ativo com api key)
- [x] Testes E2E: via inspeção do transcript JSONL (equivalente E2E definido na techspec — TestSprite não aplicável)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent_test.go` (modificado — teste de turno completo com Jev desabilitado)
- `internal/tui/tui_test.go`, `internal/tui/commands_test.go` (modificados — checagens de persistência remanescentes)
- `internal/config/config_test.go` (referência — fixture legada)
- `spec/tasks/015-prd-controle-do-jev/prd.md` (referência — métricas de sucesso)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência — "Abordagem de Testes")
