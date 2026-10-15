# Tarefa 6.0: Descoberta — tela `/config` com 6 inputs, status bar `jev`/`jev off`, `/help` e sugestões

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Descoberta: `/config`, `--doctor` (já na task 3.0), help e status bar

## Dependências

- 4.0, 5.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Tornar o estado do roteamento visível em todos os pontos de contato da TUI: a tela `/config` ganha os inputs `jev enabled (yes/no)` e `model` (6 inputs no total), salvos junto com os demais via `saveConfig()` (tui.go:1245) com validação de modelo antes de salvar; a status bar (`hintBar`, tui.go:1412) ganha indicador de roteamento derivado de `m.cfg` (`jev` / `jev off`); `/help` documenta `/jev` e o `/model` persistente; `/jev` entra nas sugestões de `nativeCommands` (commands.go:18). Com esta task o REQ-004 fecha (o `--doctor` já foi coberto na task 3.0).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês, receivers por valor na TUI, validação nos pontos de salvamento, api_key continua mascarada no `/config` (`display()`).
</skills>

<requirements>
- Tela `/config` com 6 inputs: acrescenta `jev enabled (yes/no)` e `model` aos 4 existentes; salvos junto com os demais em `saveConfig()`
- `saveConfig()` valida o modelo contra o catálogo antes de salvar; inválido → `errorBox` e mantém a tela de config; salvo válido → `rebuildAgent()` (que aplica `SetPinned(cfg.Model)`)
- Status bar com indicador de roteamento derivado de `m.cfg`: `jev` quando `JevEnabled() && api_key != ""`, `jev off` quando desabilitado; `pinned <model>` existente cobre o modelo fixo (tui.go:1447)
- `/help` ganha linha do `/jev` e atualiza a descrição de `/model` (persistente)
- `/jev` entra em `nativeCommands` (commands.go:18) — aparece nas sugestões ao digitar `/`
- Nenhum dado sensível novo: `enabled` e `model` são valores não-sensíveis; api_key permanece mascarada
</requirements>

## Subtarefas

- [x] 6.1 Estender a tela `/config`: `newConfigInputs()` com 6 inputs (`jev enabled (yes/no)` e `model`) e `saveConfig()` mapeando os 6, com validação de modelo antes de salvar
- [x] 6.2 Indicador de roteamento na status bar (`hintBar`): `jev`/`jev off` derivado de `m.cfg`, mantendo o `pinned <model>` existente
- [x] 6.3 `/help` com linha do `/jev` e descrição atualizada de `/model`; `/jev` em `nativeCommands`
- [x] 6.4 Testes de integração: `/config` com modelo inválido → erro e permanência na tela; salvamento válido persiste e re-wira; status bar distingue os três estados; `/help` e sugestões listam `/jev`

## Detalhes de Implementação

Consulte techspec.md — seção "Pontos de Persistência" (`saveConfig()`, status bar, sugestões). Precedentes: `saveConfig()` já chama `rebuildAgent()` (tui.go:1253), indicador `pinned` já renderizado em tui.go:1447, sugestões via `prefixMatches(nativeCommands, ...)` (commands.go:68).

## Critérios de Sucesso

- `go test ./internal/tui/` verde
- `/config` salva `jev enabled` e `model` junto com os demais campos; modelo inválido → `errorBox` e nada persiste
- Status bar distingue Jev ativo, Jev desabilitado e modelo fixo — critério de aceite REQ-004
- `/help` lista `/jev` e descreve o `/model` persistente — critério de aceite REQ-004
- `/jev` aparece nas sugestões de comando ao digitar `/` — critério de aceite REQ-004
- Config legada aberta e salva pelo `/config` materializa os campos novos sem quebrar os existentes

## Testes da Tarefa

- [ ] Testes de unidade: N/A (render e comandos de TUI; cobertura via integração)
- [x] Testes de integração (`tui_test.go`/`commands_test.go`):
  - [x] `/config` com modelo inválido → `errorBox` e permanece na tela de config (arquivo inalterado)
  - [x] `/config` válido → `enabled` e `model` persistidos (re-`Load` e conferir) + `rebuildAgent()` executado
  - [x] Status bar: Jev ativo → `jev`; desabilitado → `jev off`; com modelo fixo → `pinned <model>` (além do indicador de roteamento)
  - [x] `/help` contém `/jev` e a descrição nova de `/model`
  - [x] Sugestões ao digitar `/je` incluem `/jev`
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — `saveConfig()`, `newConfigInputs()`, `hintBar()`, `/help`)
- `internal/tui/commands.go` (modificado — `nativeCommands`)
- `internal/tui/tui_test.go`, `internal/tui/commands_test.go` (modificados)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência)
