# Review — Task 6.0: Descoberta — tela `/config` com 6 inputs, status bar `jev`/`jev off`, `/help` e sugestões

**Veredito: APROVADO**

## Escopo revisado

Somente `internal/tui` (tui.go, commands.go, tui_test.go, commands_test.go), conforme delimitado na task. Nenhuma mudança em `main.go` ou em pacotes fora de `internal/tui`. O working tree contém também as alterações ainda não commitadas das tasks 1.0–5.0 (branch da feature inteira em andamento); esta review cobre exclusivamente os deltas da 6.0.

## Evidências do diff

### 6.1 — Tela `/config` com 6 inputs + validação antes de salvar

- `newConfigInputs()` (tui.go:245): acrescenta `jevEnabled` (placeholder `yes/no`, `CharLimit 3`, valor de `boolToYesNo(cfg.Typesafe.JevEnabled())`) e `model` (placeholder `model name (empty = auto)`, valor de `cfg.Model`) aos 4 inputs existentes. Retorna 6 inputs na ordem url, llmKey, jevKey, skipTLS, jevEnabled, model.
- `configView()` (tui.go:1581): labels estendidos para 6 (`Jev enabled (yes/no)`, `Model`); renderiza `errorBox(m.cfgError)` entre os inputs e a linha de ajuda quando há erro de validação.
- `saveConfig()` (tui.go:1266): valida o modelo contra o catálogo (`m.catalog.Get`) **antes de qualquer mutação de `cfg`** — inválido → `m.cfgError = "unknown model X — not in catalog"` e `return` precoce (state permanece `stateConfig`, nada persiste em arquivo ou memória); válido → mapeia os 6 inputs (`Typesafe.Enabled = &enabled`, `cfg.Model = model`), `Save()`, `rebuildAgent()` (que aplica `SetPinned(cfg.Model)` via `Wire`, tui.go:1304-1307).
- Abertura de `/config` (tui.go:1069) limpa `cfgError` (sem erro stale ao reabrir a tela).
- Campo novo `cfgError string` no `Model` (tui.go:131) — receivers por valor preservados.

### 6.2 — Indicador de roteamento na status bar

- `hintBar()` (tui.go:1471-1475): `jev` (primaryStyle) quando `JevEnabled() && api_key != ""`; `jev off` (warningStyle) caso contrário (incluindo sem api key — consistente com `showJevStatus`). Posicionado imediatamente antes do indicador `pinned <model>` existente (tui.go:1476), que foi mantido intacto.
- Fixture `legacyHintBar` (tui_test.go) espelhado com o mesmo bloco — necessário para `TestSDDViewByteIdenticalToPreSidebar` continuar byte-identical.

### 6.3 — `/help` e sugestões

- `/help` (tui.go:1042-1046): linha nova `/jev on|off — toggle Jev routing · /jev — show state`; `/model` atualizado para `pin a model, persisted across restarts`.
- `nativeCommands` (commands.go:19): `/jev` inserido em posição ordenada — aparece nas sugestões ao digitar `/` e `/je`.

### 6.4 — Testes de integração

- `TestConfigInvalidModelKeepsScreenAndFile` (commands_test.go): modelo inválido → permanece em `stateConfig`, arquivo byte-a-byte inalterado, `cfg.Model`/`cfg.Typesafe.Enabled` não mutados em memória, errorBox visível na tela de config, sem re-wiring; cobre também a recuperação (corrigir o modelo → salva, limpa o erro, aplica o pin).
- `TestConfigSavePersistsJevAndModel` (commands_test.go): salvar `no` + `glm-5.3` → re-`Load` confirma `enabled=false` e `model="glm-5.3"` com os campos existentes intactos; `rebuildAgent()` executado (router vira `*router.FixedRouter`, fallback nil, `Pinned() == "glm-5.3"`).
- `TestConfigSaveMaterializesLegacyFields` (commands_test.go): config legada sem `enabled`/`model` abre com `yes`/vazio nos inputs novos; salvar materializa `enabled = true` e `model = ""` no TOML sem quebrar os campos legados.
- `TestHintBarShowsRoutingState` (tui_test.go): Jev ativo → `jev`; desabilitado → `jev off`; sem api key → `jev off`; com pin → `pinned glm-5.3` coexiste com o indicador de roteamento nos dois estados.
- `TestHelpDocumentsJevAndPersistentModel` + `TestCommandPopupSuggestsJev` (commands_test.go): `/help` contém a linha do `/jev` e a descrição persistente de `/model`; digitar `/je` sugere exatamente `[/jev]`.
- Listas esperadas de `TestCommandPopupListsNativeAndKspec` e `TestCommandPopupWithoutStoreListsNativesOnly` atualizadas com `/jev`.

## Conformidade com padrões (AGENTS.md / techspec)

- **Sem comentários no código**: `grep -n "^\s*//" internal/tui/tui.go internal/tui/commands.go` → vazio. ✓
- **Código em inglês**: identificadores, mensagens e textos da TUI em inglês. ✓
- **Receivers por valor na TUI**: `saveConfig`, `hintBar`, `configView` seguem value receiver; `rebuildAgent` pointer receiver como no código pré-existente. ✓
- **Validação nos pontos de salvamento**: modelo validado contra o catálogo em `saveConfig()` antes de salvar (a config não conhece o catálogo — direção de dependência preservada). ✓
- **api_key mascarada**: inputs de LLM/Jev API key permanecem `EchoMode: textinput.EchoPassword`; `display()` do `--doctor` intocado; nenhum dado sensível novo (`enabled` e `model` são não-sensíveis). ✓
- **Sem scope creep**: `emptyState()` (texto "Jev will pick the best model…") e sidebar não alterados — fora do escopo da task.

## Observações (não-bloqueantes)

1. **Interpretação do "errorBox" no caminho inválido**: o erro é renderizado *dentro* da tela de config (campo `cfgError` + `errorBox()` em `configView()`), em vez de anexado aos blocks do chat. Anexar aos blocks deixaria o erro invisível até o usuário sair da tela; renderizar na própria tela satisfaz "errorBox e mantém a tela de config" com o usuário vendo o feedback onde corrige. Nada persiste (arquivo e memória intocados — coberto por teste).
2. **Fixture `legacyHintBar`** em tui_test.go atualizado para espelhar o indicador novo — manutenção esperada do espelho do hintBar que o teste byte-identical exige.
3. `emptyState()` ainda menciona "Jev will pick the best model for every single call" quando não há gateway configurado — texto pré-existente, fora do escopo da task; sugere ajuste futuro (não é critério da REQ-004).

## Checks executados

| Comando | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio (nenhum arquivo listado) |
| `go test ./internal/tui/` | ✅ ok (3.5s, todos verdes incluindo os 7 testes novos/alterados) |
| `go test ./...` | ✅ ok (todos os pacotes) |

## Conclusão

REQ-004 fecha para a TUI: `/config` com 6 inputs e validação, status bar distinguindo Jev ativo/desabilitado/modelo fixo, `/help` documentando `/jev` e o `/model` persistente, `/jev` nas sugestões. O `--doctor` já havia sido coberto na task 3.0. Task 6.0 **aprovada**.
