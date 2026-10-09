# Tarefa 4.0: agent — timeout de subagente stall-based

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Timeout de subagente stall-based

## Dependências

- Nenhuma (paralela com 1.0/2.0/3.0; beneficia-se delas em runtime, mas não há dependência de código)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Trocar a semântica do timeout de subagente de relógio de parede total (`context.WithTimeout` de 5m em `runSubagent`) por watchdog de stall: timer de `subagent_timeout` (default 10m) resetado a cada evento drenado do canal `Events` do subagente; silêncio pela janela → cancela com flag distinguível. Mensagem de resultado dinâmica.

<skills>
### Conformidade com Skills Padrões

Padrões do código-base Go (AGENTS.md): sem comentários, eventos via canal, `sync` para flag atômica, testes de agent com mock gateway existente.
</skills>

<requirements>
- Default `subagentTimeout` muda de 5m para 10m; setter público `SetSubagentTimeout(time.Duration)`
- Watchdog: `time.AfterFunc` resetado no drain loop a cada evento recebido de `sub.Events`
- Disparo marca flag atômica `stalled` e cancela ctx derivado — `ctx.Err()` sozinho não distingue stall de Esc
- Resultado com stall: `error: subtask stalled for 10m0s without progress` (dinâmico com o valor configurado)
- Cancelamento do pai (Esc) segue o caminho de abort existente, inalterado
- Sem timer/goroutine vazando (stop no encerramento)
</requirements>

## Subtarefas

- [ ] 4.1 Substituir o `context.WithTimeout` total em `runSubagent` por ctx derivado + watchdog resetável; flag `stalled` atômica
- [ ] 4.2 Resetar o timer no drain goroutine existente a cada evento
- [ ] 4.3 Mensagem dinâmica de stall no resultado do subtask; garantir que abort do pai não vira mensagem de stall
- [ ] 4.4 `SetSubagentTimeout` + default 10m propagado para subagentes filhos (campo já copiado em `newSubagent`/`RunSyncPersona`)
- [ ] 4.5 Testes: adaptar `TestSubagentTimeout` (silencioso → "stalled for … without progress"); subagente com progresso contínuo (chunks espaçados) sobrevive além da janela antiga; Esc durante subagente continua abortando o turno pai

## Detalhes de Implementação

Ver techspec.md — seção "Stall de subagente (em `agent.runSubagent`)". A janela precisa cobrir o `bashTimeout` de 120s sem eventos — 10m default absorve. Nos testes usar `ag.subagentTimeout` curto (o campo já é acessível nos testes do pacote, precedente em `TestSubagentTimeout`).

## Critérios de Sucesso

- Subagente que emite progresso continuamente sobrevive além de 10m de parede
- Subagente silencioso é abortado na janela com a nova mensagem
- Esc aborta tudo como antes (regressão verde)
- `go test ./internal/agent/` verde com `-race`

## Testes da Tarefa

- [ ] Testes de unidade: watchdog de stall com mocks que bloqueiam/emitem (subtarefa 4.5)
- [ ] Testes de integração: subagente com tool call interno sob nova semântica
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go`
- `internal/agent/agent_test.go`
