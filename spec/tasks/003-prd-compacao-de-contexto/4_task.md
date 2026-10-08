# Tarefa 4.0: Linha de compação na TUI

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Evento de compação

## Dependências

- 2.0 (`EventCompaction` sendo emitido pelo agent)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

A visibilidade da compação para o usuário: a TUI trata o novo `EventCompaction` renderizando a linha sutil `⚡ context compacted (12.4k → 3.1k tokens)` em `colTextMuted`, sem interromper a leitura do chat. É o único sinal visual da compação — nenhuma pergunta, nenhuma pausa (a compação é invisível no fluxo normal, conforme o PRD).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib + dependências de TUI existentes.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (linha visível em cor muted), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; paleta existente (sem cores novas).
</skills>

<requirements>
- Caso `EventCompaction` no `handleAgentEvent` da TUI.
- Linha `⚡ context compacted (Xk → Yk tokens)` em `colTextMuted` (formatação k para milhares, ex.: 12.4k).
- Nenhuma interação: a linha é informativa apenas.
- Paleta existente — nenhuma cor nova.
</requirements>

## Subtarefas

- [ ] 4.1 Adicionar o caso `EventCompaction` ao tratamento de eventos da TUI
- [ ] 4.2 Renderizar a linha `⚡ context compacted (Xk → Yk tokens)` em `colTextMuted`
- [ ] 4.3 Escrever teste de renderização da linha (ver Testes da Tarefa)

## Detalhes de Implementação

- Formato da linha e cor: techspec, seção **Arquitetura do Sistema** (componente `internal/tui/tui.go`) e PRD, seção **REQ-004**.
- Experiência do usuário (linha sutil, sem interromper a leitura): PRD, seção **Experiência do Usuário**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: `EventCompaction` com `TokensBefore/TokensAfter` → render contém `⚡ context compacted (` em `colTextMuted` com os valores formatados.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`):
  - Renderização da linha de compação — `EventCompaction{TokensBefore, TokensAfter}` → linha `⚡ context compacted (Xk → Yk tokens)` visível em muted (techspec, seção **Arquivos relevantes**: `internal/tui/tui_test.go`).
- [ ] Testes de integração — cobertos pelos testes de agent da task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa` (linha visível em sessão longa real).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — caso `EventCompaction` + linha muted
- `internal/tui/tui_test.go` — teste de renderização
