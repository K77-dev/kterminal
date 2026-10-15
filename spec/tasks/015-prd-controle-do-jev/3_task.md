# Tarefa 3.0: main.go — wiring via `Wire()`, validação fatal de modelo no startup e `--doctor` com estado do Jev

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Modelo fixo determinístico (validação contra o catálogo no wiring do startup; `FixedRouter` como primary via `Wire()`)
- REQ-004 — Descoberta (`--doctor` imprime `jev enabled` e o modelo fixo efetivo; sem probe do endpoint quando desabilitado)

## Dependências

- 1.0, 2.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Substituir o bloco de wiring de main.go:82-90 por `router.Wire(cfg.Typesafe.APIKey, cfg.Typesafe.JevEnabled(), cfg.Model, cat)`, validar `cfg.Model` contra o catálogo no startup (modelo fora do catálogo → erro fatal apontando o valor) e estender o `--doctor` (main.go:181) com o estado do Jev e o modelo fixo. Com esta task, o comportamento de startup já muda conforme REQ-002: usuários sem api_key passam do `HeuristicRouter` por keywords para o `FixedRouter` no default do catálogo (mudança intencional, ver "Arquitetura" da techspec). A duplicação na TUI (`rebuildAgent`) só é eliminada na task 4.0.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês, validação nos pontos de wiring (não no parsing da config), erros de startup → stderr + exit 1 apontando o valor rejeitado.
</skills>

<requirements>
- Wiring de main.go:82-90 substituído por `router.Wire(cfg.Typesafe.APIKey, cfg.Typesafe.JevEnabled(), cfg.Model, cat)`; par resultante passado ao `agent.New` como hoje (main.go:95)
- `cfg.Model` aplicado como pin no agente principal via `SetPinned(cfg.Model)` no ponto de construção (fluxo da techspec: config → `Wire()` → `SetRouters` + `SetPinned`)
- Validação de `cfg.Model` contra o catálogo no startup: nome fora do catálogo → stderr + exit 1 apontando o valor; vazio → segue automático
- `runDoctor` imprime `jev enabled: yes/no` na seção de config e `model: <name>` ou `model: auto`
- Seção Jev do `--doctor`: com Jev desabilitado imprime `jev: disabled` **sem probe** do endpoint Typesafe (zero chamadas quando desabilitado)
- `SetPinned`/`SetRouters` consumidos como estão — `internal/agent` sem mudanças
</requirements>

## Subtarefas

- [x] 3.1 Substituir o bloco de wiring de main.go:82-90 por `router.Wire(...)` e aplicar `ag.SetPinned(cfg.Model)` após a construção do agente
- [x] 3.2 Validar `cfg.Model` contra o catálogo no startup: inválido → erro fatal apontando o valor; adicionar teste em `main_test.go`
- [x] 3.3 Estender `runDoctor` (main.go:181): `jev enabled: yes/no`, `model: <name>|auto`, e seção Jev `jev: disabled` sem probe quando desabilitado
- [x] 3.4 Testes de integração em `main_test.go`: startup com modelo inválido → exit com erro; `runDoctor` com `enabled=false` não faz requisição HTTP ao endpoint Typesafe (httptest como testemunha)

## Detalhes de Implementação

Consulte techspec.md — seções "Arquitetura do Sistema" (componentes `main.go` e fluxo de dados), "Pontos de Persistência" (`--doctor`) e "Abordagem de Testes" (testes de `main_test.go`). O padrão de erro fatal de startup já existe em main.go:60-63 (`kterminal: <err>` + exit 1).

## Critérios de Sucesso

- `go build ./... && go vet ./... && go test ./...` verde
- Startup com `model` fora do catálogo → boot falha com erro apontando o valor (critério de aceite REQ-002)
- `--doctor` reflete `enabled` e o modelo fixo (critério de aceite REQ-004)
- `--doctor` com `enabled=false` → nenhuma requisição HTTP ao endpoint Typesafe
- Startup com config legada (sem campos novos) → par de routers idêntico ao comportamento atual

## Testes da Tarefa

- [x] Testes de unidade: cobertos pelos testes de `Wire()`/`FixedRouter` da task 2.0 (nada novo isolado aqui)
- [x] Testes de integração (`main_test.go`):
  - [x] Startup com `model` fora do catálogo → exit com erro cuja mensagem contém o valor rejeitado
  - [x] `runDoctor` com `enabled=false` → saída contém `jev: disabled` e o servidor httptest do endpoint Typesafe recebe zero requisições
  - [x] `runDoctor` com `enabled=true` + api key → saída contém `jev enabled: yes` e `model: auto|<name>` conforme a config
- [ ] Testes E2E: N/A (verificado via transcript na task 7.0)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go` (modificado — wiring, validação de startup, `runDoctor`)
- `main_test.go` (modificado)
- `internal/router/router.go` (referência — `Wire()` da task 2.0)
- `internal/agent/agent.go` (referência — `SetPinned` agent.go:145, `SetRouters` agent.go:154, sem mudanças)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência)
