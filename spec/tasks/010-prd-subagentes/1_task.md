# Tarefa 1.0: Extrair `runLoop` do loop do Agent (refactor puro)

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Subagente isolado (fundação: o motor de turnos reusável que o subagente consumirá)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Refactor preparatório, sem mudança de comportamento: a lógica do loop de turnos existente (`loop`) é extraída para `runLoop(ctx) (finalText string, err error)` — um único motor de turnos com dois modos de acionamento futuros: async com eventos de turno (o `loop` atual vira wrapper com goroutine + eventos de turno) e sync com coleta (o `RunSync` da task 2.0). Steps, roteamento, abort e tool calls ficam idênticos nos dois níveis — menos código, menos drift. Critério de conclusão: **todos os testes existentes do agent permanecem verdes sem alteração de semântica**.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante); refactor puro — zero mudança de comportamento.
</skills>

<requirements>
- Extrair a lógica do loop de turnos para `runLoop(ctx) (finalText string, err error)` reusável.
- `loop` existente vira wrapper: goroutine + eventos de turno (`EventTurnStart`/`EventTurnDone`/`EventTurnAborted`), delegando o motor a `runLoop`.
- Nenhuma mudança de comportamento observável: mesmos eventos, na mesma ordem, com o mesmo conteúdo.
- Todos os testes existentes de `internal/agent` passam sem modificação.
</requirements>

## Subtarefas

- [x] 1.1 Extrair o corpo do loop de turnos para `runLoop(ctx) (finalText, error)`
- [x] 1.2 Reduzir o `loop` existente a wrapper (goroutine + eventos de turno) chamando `runLoop`
- [x] 1.3 Rodar a suíte completa de testes do agent e confirmar tudo verde sem mudanças nos testes

## Detalhes de Implementação

- Racional da extração (um motor, dois acionamentos): techspec, seção **Considerações Técnicas → Decisões Principais** (item 5) e **Verificações Técnicas → Arquitetura**.
- Ordem de construção (refactor primeiro, testes existentes verdes): techspec, seção **Sequenciamento de Desenvolvimento** (passo 1).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Todos os testes existentes de `internal/agent` passam **sem modificação** — o refactor é puro.
- Nenhum evento novo, nenhuma mudança de assinatura pública além da interna `runLoop`.

## Testes da Tarefa

- [x] Testes de unidade — suíte existente completa de `internal/agent` passando sem mudanças (é o próprio critério do refactor puro).
- [x] Testes de integração — cobertos pela suíte existente (fluxos com mock gateway).
- [x] Testes E2E — não aplicáveis ao refactor; cenários da feature são deferidos ao `kspec-qa` após as tasks 2.0-5.0.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `runLoop` extraído; `loop` wrapper
- `internal/agent/agent_test.go` — suíte existente (sem mudanças; deve permanecer verde)
