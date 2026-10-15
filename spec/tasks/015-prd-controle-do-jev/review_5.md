# Review — Task 5.0: TUI — comando `/jev on|off|status` com re-wiring sem restart

**Veredito: APROVADO**

## Escopo executado

- Arquivos modificados (todos dentro do escopo permitido):
  - `internal/tui/commands.go` — `handleJevCommand` (parsing de `/jev`/`/jev on`/`/jev off`/estado inválido), `showJevStatus`, `enableJev` (guarda de api key), `applyJevState` (caminho único de persistência).
  - `internal/tui/tui.go` — apenas o dispatcher: `case "/jev":` em `handleCommand`, entre `/config` e `/mode` (inserção cirúrgica de 2 linhas).
  - `internal/tui/commands_test.go` — 7 testes de integração + helpers `newJevCommandModel` (XDG_CONFIG_HOME isolado, env `KTERMINAL_MODEL`/`KTERMINAL_TYPESAFE_ENABLED` neutralizados, wiring via `router.Wire` espelhando o boot com o client do Jev apontado para o mock) e `countingJev` (mock httptest do `/v1/systemone` com contador atômico).
- **NÃO tocados**: `nativeCommands`, `/help`, status bar, tela `/config` (tudo task 6.0); `main.go`; `internal/router`; `internal/config`; `tui_test.go` (nenhuma mudança de view — o picker renderizado é o da task 4.0).

## Evidências do diff × requirements

| Requirement | Evidência |
| --- | --- |
| `/jev` sem argumentos exibe estado | `showJevStatus`: `jev: on/off` + `model: X` onde X = `cfg.Model` ou `m.catalog.DefaultModel` (techspec: "modelo fixo vigente (`cfg.Model` ou default do catálogo)") |
| `/jev on` sem api key → errorBox, nada persiste | `enableJev` guarda `APIKey == ""` **antes de qualquer mutação** → `errorBox("jev needs a typesafe api key — set one in /config")`; teste compara os bytes do arquivo de config antes/depois e o ponteiro do router |
| `/jev on` com chave → `Enabled=true`, save, rebuild | `applyJevState(true)`: muta `cfg.Typesafe.Enabled` → `cfg.Save()` → `rebuildAgent()`; teste confere re-`Load` com `enabled = true` + `ag.Router` = `*router.JevRouter` + fallback `*router.HeuristicRouter` |
| `/jev off` → `Enabled=false`, save, rebuild, picker | `applyJevState(false)`: mesma ordem + `openModelPopup()` (picker da task 4.0); teste confere `modelOpen == true`, re-`Load` com `enabled = false`, `ag.Router` = `*router.FixedRouter`, fallback nil |
| `/jev off` idempotente | Caminho off não tem guarda de estado: sempre save + rebuild + picker, sem erro; `TestJevOffIdempotent` roda off duas vezes (picker reabre, config segue `enabled = false`, sem erro de save) |
| Caminho único da techspec | `applyJevState` segue exatamente mutar `m.cfg` → `m.cfg.Save()` → `m.rebuildAgent()`; em falha de `Save()` reverte `Enabled` ao ponteiro anterior (padrão do `applyModelChoice` da task 4.0) |
| Re-wiring sem restart | `rebuildAgent()` (task 4.0) via `router.Wire(...)` + `SetRouters` + `SetPinned`; `TestJevOffStopsJevRequestsInNextTurn` prova em turno real: rota 1 via `jev` (mock atingido), `/jev off` + esc, rota 2 via `fixed` no default do catálogo com **zero** novas requisições ao mock |
| Esc no picker mantém automático (PRD UX) | Coberto no fluxo do teste de turno: esc fecha o picker sem tocar `cfg.Model` → `FixedRouter` resolve o `default_model` (`glm-5.2`), assertado no evento de rota |

## Semântica de estado adotada

- `jev: on` ⇔ `JevEnabled() && api_key != ""` (estado efetivo, igual ao critério da status bar na techspec e ao "Jev ativo ⇔ enabled=true **e** api_key presente" do PRD). Sem chave → `jev: off` mesmo com `enabled = true` no arquivo — documentado em teste.
- Confirmações no vocabulário existente: `primaryStyle` "jev on"/"jev off" (precedente "mode: X") + `resultStyle` "config saved" (precedente `applyModelChoice`/`saveConfig`); erros em `errorBox` com o padrão "… — próxima ação" ("unknown jev state toggle — /jev on or /jev off").

## Testes criados (commands_test.go)

- `TestJevOnWithoutAPIKeyKeepsConfig` — errorBox, bytes do arquivo idênticos, router intacto, nenhuma confirmação
- `TestJevOffPersistsAndOpensPicker` — `enabled = false` no re-`Load` (sobrevive restart), `modelOpen`, `*FixedRouter`/fallback nil, confirmações
- `TestJevOffStopsJevRequestsInNextTurn` — turno 1 roteia via `jev` (mock atingido ≥1×); após `/jev off` + esc, turno 2 roteia via `fixed` no default do catálogo e o contador do mock não muda (critério de aceite REQ-001: zero chamadas ao Jev na sessão)
- `TestJevOnReinstallsJevRouter` — partindo de off (precondição `*FixedRouter`), `/jev on` reinstala `*JevRouter` + `*HeuristicRouter` e persiste `enabled = true`
- `TestJevOffIdempotent` — segundo off reabre o picker, persiste, sem erro
- `TestJevStatusShowsStateAndModel` — on com chave, modelo default do catálogo, modelo fixo após `/model glm-5.3`, off após `/jev off`, off sem chave
- `TestJevUnknownStateErrors` — `/jev toggle` → errorBox, arquivo nem criado

## Checks

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ OK |
| `go vet ./...` | ✅ OK |
| `gofmt -l .` | ✅ vazio (nenhum arquivo listado) |
| `go test ./internal/tui/ -count=1` | ✅ ok (4.3s) |
| `go test ./internal/tui/ -count=1 -race -run TestJev` | ✅ ok (1.6s) |
| `go test ./... -count=1` | ✅ todos os pacotes ok (agent 13s, tools 33s, tui 7s) |

Nota de ambiente: a primeira execução combinada (`build && vet && gofmt && test`) excedeu 5min sem produzir output (compilação a frio do binário de teste transiente); re-executada por etapas, todas verdes e rápidas — falha transitória de ambiente, não do código.

## Ressalvas (informativas, não bloqueantes)

1. **Status "on" exige api key** (estado efetivo) enquanto o `--doctor` da task 3.0 imprime o flag bruto (`jev enabled: yes`) e separa o caso sem chave ("NOT CONFIGURED"). Divergência intencional: `/jev` responde "qual roteador está decidindo agora", alinhado ao critério da status bar da techspec (task 6.0).
2. **`/jev on` com Jev já ligado é idempotente** (save + rebuild + confirmação "jev on"). A task só exigia idempotência para off; o caminho on sem guarda de estado é o simétrico natural e inofensivo.
3. **`/jev on` reinstala um `JevRouter` apontando para o endpoint real** (`Wire` usa `jev.New(apiKey)` com `DefaultBaseURL`) — comportamento da factory da task 2.0; o teste de on não roda turno posterior, então nenhum request sai. Apontar o router reinstalado para um mock exigiria injetar base URL no `Wire`, fora do escopo desta task.
4. **Sem guarda de busy** em `/jev` — consistente com `/model` (task 4.0), que também troca routers em runtime; o agent lê `a.Router` a cada `decide()`, então a troca mid-turn é segura no mesmo grau do precedente.

## Conformidade com padrões

- Sem comentários no código (verificado por inspeção do diff).
- Código em inglês; receivers por valor nos handlers que retornam `tea.Model` (espelho de `handleModelCommand`/`applyModelChoice`); mutações de popup via métodos de ponteiro existentes; confirmações curtas no vocabulário da TUI.
- Sem secrets: apenas valores dummy (`gw-key`, `jev-key`, `test-key`), padrão já existente.
- Escopo cirúrgico: `tui.go` só ganhou o case do dispatcher; nada das tasks 6.0/7.0 foi antecipado.
