# Review — Task 4.0: TUI — `rebuildAgent()` via `Wire()`, `/model` persistente e picker de modelos

**Veredito: APROVADO**

## Escopo executado

- Arquivos modificados (todos dentro do escopo permitido):
  - `internal/tui/tui.go` — estados `modelOpen`/`modelSelected` no `Model`; handlers do picker no `handleChatKey` (bloco espelhando `modeOpen`); case `/model` delegando a `handleModelCommand`; `rebuildAgent()` reescrito com `router.Wire()` + `SetRouters` + `SetPinned(m.cfg.Model)`; render do popup em `chatView()`; import `jev` removido (tornou-se não usado).
  - `internal/tui/commands.go` — `modelChoice`/`modelChoices()`, `closeModelPopup()`, `openModelPopup()`, `handleModelCommand()`, `applyModelChoice()`, `selectModel()`, `modelPopupView()` (espelho do popup `/mode`: mesmos estilos `chromeLabelStyle`/`mentionSelectedStyle`/`mentionItemStyle`/`mentionPopupStyle`).
  - `internal/tui/commands_test.go` — 9 testes de integração + helpers (`newModelCommandModel` com `XDG_CONFIG_HOME` isolado, `seedPinnedConfig`, `pickerIndexOf`).
  - `internal/tui/tui_test.go` — `legacyChatView` (fixture byte-identical) ganhou o render do `modelPopupView()` para continuar espelhando `chatView`.
- **NÃO tocados**: `main.go`, `main_test.go` (respeitada a execução paralela da task 3.0). `/jev` **não** implementado (fica para a task 5.0) — confirmado por grep: nenhuma ocorrência de `/jev` em `internal/tui`. `/help`, `nativeCommands` e status bar não alterados (escopo da task 6.0).

## Evidências do diff × requirements

| Requirement | Evidência |
| --- | --- |
| `rebuildAgent()` com wiring único via `Wire()` | Bloco manual (`NewJev`/`HeuristicRouter`/swap de fallback) substituído por uma única chamada `router.Wire(m.cfg.Typesafe.APIKey, m.cfg.Typesafe.JevEnabled(), m.cfg.Model, m.catalog)` + `SetRouters` + `SetPinned(m.cfg.Model)`; import `jev` removido de tui.go |
| `/model <name>` persistente com validação | `handleModelCommand` valida via `m.catalog.Get` antes de qualquer mutação; inválido → `errorBox("unknown model X — not in catalog")` e nada persiste (teste confere que o arquivo nem chega a existir) |
| Ordem exigida `cfg.Model` → `Save()` → `SetPinned` → `rebuildAgent()` | `applyModelChoice` segue exatamente esta ordem; em falha de `Save()` reverte `cfg.Model` ao valor anterior (nada persiste, nada re-wira) |
| `/model auto` limpa o campo | `applyModelChoice("")` → `cfg.Model=""`, save, `SetPinned("")`, rebuild |
| `/model` sem argumentos abre o picker | `arg == ""` → `openModelPopup()`; comportamento novo substitui a remoção de pin (nenhum teste antigo dependia do comportamento anterior — verificado por grep) |
| Picker espelha o popup `/mode` | Estados `modelOpen`/`modelSelected` no `Model` (ao lado de `modeOpen`/`modeSelected`), handlers `tab/enter/esc/up/down` no `handleChatKey` no mesmo padrão do bloco `modeOpen`, view com header `MODEL` e marcador `▸` |
| Itens `auto` + catálogo com resumo | `modelChoices()`: entrada `auto` + `fmt.Sprintf("%s · $%.2f/$%.2f · ~%.0f tok/s", ...)` (usa `MeasuredTPS` quando disponível, senão `TPSEstimate`) |
| Pre-seleção do pin corrente | `openModelPopup` procura `m.agent.Pinned()` nas choices; sem pin → índice 0 (`auto`) |
| Esc cancela sem alterar nada | `closeModelPopup` só reseta estado; teste compara os bytes do arquivo de config antes/depois |
| Enter persiste + re-wira | `selectModel` → `applyModelChoice` → save + `rebuildAgent`; teste afirma `ag.Router` vira `*router.FixedRouter` (partia de `nil`) e o re-`Load` confere o modelo |
| Pin vence com Jev habilitado | `TestModelCommandKeepsJevRoutingWithPin`: com api key, `ag.Router` é `*router.JevRouter` (fallback `*router.HeuristicRouter`) e `Pinned()` aplicado — o short-circuit de pin no `decide()` (agent.go:1040) é comportamento existente e preservado |
| Confirmação no vocabulário existente | `"model pinned: X"` (primaryStyle, precedente tui.go:1062) + `"config saved"` (resultStyle, precedente saveConfig) |

## Testes criados (commands_test.go)

- `TestModelCommandPersistsPinAndRewires` — persistência no arquivo (re-`Load`), `SetPinned`, re-wiring (`*FixedRouter`, fallback nil), confirmações
- `TestModelCommandKeepsJevRoutingWithPin` — Jev on + pin: `*JevRouter`/`*HeuristicRouter` + pin aplicado
- `TestModelCommandUnknownModelKeepsConfig` — errorBox, arquivo inexistente, cfg/pin intocados
- `TestModelAutoClearsPersistedModel` — campo `model` vazio no re-`Load` + pin removido
- `TestModelCommandWithoutArgsOpensPicker` — `modelOpen`, choices = `auto` + catálogo na ordem, label com `$`/`tok/s`, view contém header e linha `glm-5.2 · $1.00/$4.00 · ~70 tok/s`, pin não removido, nada persistido
- `TestModelPickerPreSelectsPinnedModel` — seleção no índice do pin; sem pin → 0
- `TestModelPickerEscKeepsConfig` — fecha, reseta seleção, pin intacto, bytes do arquivo idênticos
- `TestModelPickerEnterPersistsSelection` — down + enter: persiste a seleção escolhida, re-`Load` confere, `*FixedRouter` re-wired
- `TestModelPickerEnterOnAutoClearsModel` — navegar até `auto` + enter limpa pin e campo

## Checks

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ OK |
| `go vet ./...` | ✅ OK |
| `gofmt -l .` | ✅ vazio (nenhum arquivo listado) |
| `go test ./internal/tui/` | ✅ ok (3.9s, `-count=1`) |
| `go test ./internal/tui/ -run TestModel -race` | ✅ ok |
| `go test ./...` | ✅ todos os pacotes ok |

Nota de paralelismo: o primeiro `go build ./...` falhou com `undefined: validateModel` em `main.go` (task 3.0 em edição simultânea). Re-executado após alguns segundos: build verde — falha transitória confirmada, não causada por esta task. Nenhum arquivo fora do escopo foi tocado.

## Ressalvas (informativas, não bloqueantes)

1. `/model <name> <lixo>` (argumento com espaço) agora erra (`unknown model "name lixo"`), enquanto o código antigo pinava `parts[1]`. Mudança intencional: mais estrita e explícita; nomes de modelo não contêm espaço.
2. Mensagem do `auto` — `"model auto — Jev decides again"` — mantém o vocabulário anterior ("model pin removed — Jev decides again"), mas a frase assume Jev habilitado; com Jev desligado, `auto` significa default do catálogo. Refinamento de copy pode entrar na task 5.0/6.0 se desejado.
3. `applyModelChoice` reverte `cfg.Model` em memória quando `Save()` falha (defensivo; o requirement só exigia "nada persiste" para modelo inválido, que é tratado antes da mutação).
4. `legacyChatView` em `tui_test.go` foi estendido com o render do popup do modelo — necessário para manter `TestSDDViewByteIdenticalToPreSidebar` como comparação fiel de `chatView`.
5. Pre-seleção usa `m.agent.Pinned()` (pin corrente), não `m.cfg.Model` — fiel ao techspec ("pre-seleciona o pin corrente"); em sessão real ambos coincidem pois o wiring da task 3.0 e o `rebuildAgent` aplicam `SetPinned(cfg.Model)`.

## Conformidade com padrões

- Sem comentários no código (único `//` restante em tui.go é a URL dentro de string de placeholder).
- Código em inglês; receivers por valor nos métodos que retornam `tea.Model` (espelho de `handleCommand`/`selectMode`) e ponteiro nos mutadores (espelho de `openModePopup`/`closeModePopup`); `strings.Builder` no padrão do `modePopupView`.
- Sem secrets: apenas valores dummy de teste (`gw-key`, `jev-key`), padrão já existente.
