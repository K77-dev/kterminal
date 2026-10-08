# Tarefa 4.0: Renderização do diff na TUI: cores, truncamento e `confirmView`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Diff pós-execução
- REQ-002 — Diff antes da aprovação (modo `--confirm`)

## Dependências

- 3.0 (eventos `EventToolResult`/`EventConfirm` carregando o diff)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Esta task fecha o ciclo na interface: o tema ganha duas cores novas (`colDiffAdded #4fd6be`, `colDiffRemoved #c53b53` — paleta do tema original do opencode) e a TUI renderiza o bloco de diff imediatamente após a linha de resultado da tool, com prefixo `+`/`-`/espaço e truncamento no meio para diffs longos (~40 linhas visíveis com `… N more lines …`). No modo `--confirm`, `confirmView()` exibe o diff completo antes do y/n. Diff vazio não renderiza bloco.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib + dependências de TUI existentes.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (diff visível no chat/confirm), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; `strings.Builder` por ponteiro.
</skills>

<requirements>
- `internal/tui/theme.go`: `colDiffAdded #4fd6be`, `colDiffRemoved #c53b53`.
- Bloco de diff renderizado imediatamente após a linha de resultado da tool (`EventToolResult`).
- Prefixos: `+` em `colDiffAdded`, `-` em `colDiffRemoved`, espaço (contexto) em `colTextMuted`.
- Truncamento: > 40 linhas → primeiras ~20 e últimas ~20 com `… N more lines …` em `colTextMuted` no meio.
- `confirmView()` exibe o diff antes do hint y/n (v1 usa o mesmo truncamento de 40 linhas).
- Diff vazio → nenhum bloco extra renderizado.
</requirements>

## Subtarefas

- [x] 4.1 Adicionar `colDiffAdded` e `colDiffRemoved` ao tema
- [x] 4.2 Renderizar o bloco de diff após a linha de resultado com cores por prefixo
- [x] 4.3 Implementar o truncamento no meio (~40 linhas visíveis + `… N more lines …`)
- [x] 4.4 Exibir o diff dentro de `confirmView()` antes do y/n
- [x] 4.5 Escrever os testes de renderização 12-15 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Cores e regras de renderização (prefixos, truncamento, `confirmView`): techspec, seção **Design de Implementação → Renderização na TUI**.
- Experiência do usuário (bloco imediato, truncamento legível): PRD, seção **Experiência do Usuário**.
- Diff vazio: sem bloco na TUI (techspec, seção **Arquitetura do Sistema → Fluxo de dados**, item 3).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Testes provam: bloco contém ANSI das cores novas; diff de 100 linhas → ~40 visíveis + indicador; diff vazio → sem bloco; `confirmView` mostra linhas `+`/`-` antes do hint y/n.
- O diff aparece imediatamente após a linha da tool, sem interação adicional.

## Testes da Tarefa

- [x] Testes de unidade (`internal/tui/tui_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 12-15):
  - `TestDiffBlockRendersColors` — `EventToolResult` com diff → bloco contém ANSI de `colDiffAdded` (`\x1b[38;2;79;214;190`) e `colDiffRemoved`.
  - `TestDiffBlockTruncatesMiddle` — diff de 100 linhas → ~40 visíveis + `… N more lines …`.
  - `TestEmptyDiffRendersNoBlock` — diff vazio → nenhum bloco extra.
  - `TestConfirmViewShowsDiff` — `pendingConfirm` com diff → view contém linhas `+`/`-` antes do hint y/n.
- [x] Testes de integração — cobertos pelos testes de agent da task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa` (edit real visível como diff colorido; `--confirm` com diff antes do y/n; arquivo grande truncado).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/theme.go` — `colDiffAdded #4fd6be`, `colDiffRemoved #c53b53`
- `internal/tui/tui.go` — bloco de diff, truncamento, `confirmView` com diff
- `internal/tui/tui_test.go` — testes de renderização (cores, truncamento, confirm)
