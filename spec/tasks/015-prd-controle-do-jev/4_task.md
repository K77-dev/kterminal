# Tarefa 4.0: TUI — `rebuildAgent()` via `Wire()`, `/model` persistente e picker de modelos

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Picker de modelos e `/model` persistente (popup no estilo `/mode`, `/model auto`, semântica de pin preservada)

## Dependências

- 1.0, 2.0

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Eliminar a duplicação de wiring na TUI: `rebuildAgent()` (tui.go:1261) passa a usar `router.Wire(m.cfg.Typesafe.APIKey, m.cfg.Typesafe.JevEnabled(), m.cfg.Model, m.catalog)` + `SetRouters` + `SetPinned(cfg.Model)`. Tornar o `/model` persistente (`/model <name>` grava `cfg.Model` na config; `/model auto` limpa; `/model` sem argumentos abre o picker — comportamento novo) e criar o picker de modelos espelhando o popup `/mode` (commands.go:120-170): entrada `auto` + modelos do catálogo com resumo de preço/velocidade, navegação por setas, enter seleciona (persiste + re-wira), esc cancela sem alterar. Esta task entrega REQ-003 completo; o `/jev` (que reusa o picker) fica na task 5.0.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês, receivers por valor na TUI, `strings.Builder` por ponteiro, eventos do agente via canal. Picker segue o precedente do popup `/mode` (estados, handlers, view).
</skills>

<requirements>
- `rebuildAgent()` substitui o bloco duplicado (tui.go:1265-1281) por `router.Wire(...)` + `m.agent.SetRouters(...)` + `m.agent.SetPinned(m.cfg.Model)`
- `/model <name>`: valida contra o catálogo → `cfg.Model = name`, `cfg.Save()`, `SetPinned(name)`, `rebuildAgent()`; inválido → `errorBox`
- `/model auto`: `cfg.Model = ""`, save, `SetPinned("")`, rebuild (Jev volta a decidir se habilitado; senão default do catálogo)
- `/model` sem argumentos: abre o picker (comportamento novo — hoje remove o pin)
- Picker com estados `modelOpen`/`modelSelected` + `openModelPopup()`/`selectModel()`/`modelPopupView()`, espelhando o popup `/mode` (commands.go:120-170)
- Itens do picker: entrada `auto` + modelos do catálogo com resumo `$in/$out · ~tok/s`; pre-seleciona o pin corrente quando definido
- Setas navegam, enter seleciona (persiste + rebuild), esc cancela sem alterar nada
- Com Jev habilitado e modelo fixo definido, o modelo fixo continua vencendo (semântica de pin atual); confirmação curta no vocabulário existente ("model pinned: X", "config saved")
</requirements>

## Subtarefas

- [x] 4.1 Rework de `rebuildAgent()` (tui.go:1261): `router.Wire(...)` + `SetRouters` + `SetPinned(m.cfg.Model)`
- [x] 4.2 `/model <name>` persistente com validação contra o catálogo e `/model auto` limpando o campo (commands.go)
- [x] 4.3 `/model` sem argumentos abre o picker: estados `modelOpen`/`modelSelected` no Model (tui.go:115-116), handlers de teclas no `Update` (tui.go:629-638) e `openModelPopup()`/`selectModel()`
- [x] 4.4 `modelPopupView()` com entrada `auto` + modelos do catálogo (nome + `$in/$out · ~tok/s`), pre-seleção do pin corrente, estilos do popup `/mode`
- [x] 4.5 Testes de integração em `commands_test.go`/`tui_test.go`: persistência de `/model <name>` (re-`Load` e conferir), `/model auto` limpa, esc não altera nada, enter persiste e re-wira

## Detalhes de Implementação

Consulte techspec.md — seções "Comandos TUI" (tabela de comandos e especificação do picker) e "Pontos de Persistência" (`rebuildAgent()`). Precedentes: popup `/mode` (commands.go:120-170), confirmação "model pinned: X" (tui.go:1062), `saveConfig()` chamando `rebuildAgent()` (tui.go:1253).

## Critérios de Sucesso

- `go test ./internal/tui/` verde
- `/model <name>` persiste na config e sobrevive restart (re-`Load` confere) — critério de aceite REQ-003
- `/model auto` limpa o campo `model` na config — critério de aceite REQ-003
- Picker: esc não altera nada; enter persiste a seleção e re-wira — critério de aceite REQ-003
- `rebuildAgent()` não contém mais wiring manual duplicado (uma única chamada a `router.Wire`)
- Modelo inválido em `/model <name>` → `errorBox` e nada persiste

## Testes da Tarefa

- [x] Testes de unidade: cobertos por `modelPopupView` via testes de render existentes (padrão do popup `/mode`)
- [x] Testes de integração (`commands_test.go`/`tui_test.go`):
  - [x] `/model <name>` → `cfg.Model` persistido no arquivo (re-`Load` e conferir) + `SetPinned` aplicado
  - [x] `/model <name>` com nome fora do catálogo → `errorBox`, arquivo inalterado
  - [x] `/model auto` → campo `model` vazio na config + pin removido
  - [x] `/model` sem argumentos → `modelOpen == true` com itens `auto` + catálogo
  - [x] Picker: esc fecha sem alterar config; enter persiste a seleção e re-wira (routers trocados)
  - [x] Pre-seleção do picker aponta para o pin corrente quando definido
- [ ] Testes E2E: N/A (inspeção de transcript na task 7.0)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — estados do picker, `rebuildAgent()`, handlers no `Update`)
- `internal/tui/commands.go` (modificado — `/model`, `openModelPopup()`, `selectModel()`, `modelPopupView()`)
- `internal/tui/commands_test.go`, `internal/tui/tui_test.go` (modificados)
- `internal/router/router.go` (referência — `Wire()` da task 2.0)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência)
