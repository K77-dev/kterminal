# Tarefa 4.0: Campo `Depth` na session e renderização aninhada na TUI

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Eventos aninhados
- REQ-006 — TUI e transcript

## Dependências

- 2.0 (eventos com `Depth`/`ParentTool` chegando ao canal do principal)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

A visibilidade da delegação: na TUI, eventos com `Depth > 0` renderizam com prefixo de 2 espaços por nível e estilos em `colSecondary` (linha de rota, tool, resultado) — o fluxo do principal mantém os estilos atuais; a linha da tool `task` usa o estilo de tool normal (a indentação começa nos eventos **do** subagente). O hint bar mostra `subagent running` enquanto há subagente ativo — flag no Model setada por `EventToolStart` da tool `task` e limpa no `EventToolResult` dela. O campo `Depth` no `session.Event` (`omitempty`) já é gravado pela task 2.0; aqui a TUI consome os eventos aninhados. Pode rodar em paralelo com a task 3.0.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib + dependências de TUI existentes.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (fluxo indentado distinguível), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; paleta existente (`colSecondary`).
</skills>

<requirements>
- `handleAgentEvent`: `e.Depth > 0` → prefixo de 2 espaços por nível + estilos em `colSecondary` (rota, tool, resultado); fluxo do principal inalterado.
- A linha da tool `task` usa o estilo de tool normal — indentação começa nos eventos do subagente.
- Hint bar: `subagent running` enquanto `busy` e houver subagente ativo — flag setada por `EventToolStart{Tool: "task"}`, limpa no `EventToolResult` dela.
- Eventos do fluxo principal permanecem visualmente idênticos aos atuais.
</requirements>

## Subtarefas

- [ ] 4.1 Tratar `Depth > 0` no `handleAgentEvent` com indentação de 2 espaços por nível e `colSecondary`
- [ ] 4.2 Implementar a flag de subagente ativo e o hint bar `subagent running`
- [ ] 4.3 Escrever os testes 10-11 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Regras de renderização aninhada e hint bar: techspec, seção **Design de Implementação → Renderização na TUI**.
- Decisão "re-emissão no canal principal com Depth" (um consumidor na TUI): techspec, seção **Considerações Técnicas → Decisões Principais** (item 3).
- Transcript com `depth: 1` no mesmo JSONL: techspec, seção **Monitoramento e Observabilidade → Logging Estruturado** (gravado pela task 2.0).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: `EventRoute`/`EventToolStart` com `Depth: 1` → blocos com prefixo de 2 espaços em `colSecondary`.
- Teste prova: `EventToolStart{Tool: "task"}` → hint contém `subagent running`; `EventToolResult` da task → some.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 10-11):
  - `TestNestedEventsRenderIndented` — `Depth: 1` → prefixo de 2 espaços em `colSecondary`.
  - `TestHintBarShowsSubagentRunning` — hint com `subagent running` durante a task; some no resultado.
- [ ] Testes de integração — cobertos pelos testes de agent da task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa` (delegação real visível indentada no chat; hint bar durante a execução).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — renderização indentada `colSecondary`, hint bar `subagent running`
- `internal/tui/tui_test.go` — indentação, hint bar
- `internal/session/session.go` — campo `Depth` gravado (task 2.0)
