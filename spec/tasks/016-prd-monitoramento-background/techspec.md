# Tech Spec — Monitoramento de processos em background e status bar enxuta

## Requisitos Atendidos

- REQ-001 — Lançamento em background via tool `bash` (parâmetro `run_in_background`, retorno imediato, erro de spawn, múltiplos monitoramentos, saída descartada, prompt do sistema atualizado)
- REQ-002 — Contador dinâmico `bg N` no rodapé (sempre visível, tempo real, oculto com zero monitoramentos)
- REQ-003 — Seletor interativo `/bg` para parar monitoramentos individuais (escalonamento SIGTERM → SIGKILL)
- REQ-004 — Encerramento de todos os monitoramentos na saída do kterminal (zero órfãos)
- REQ-005 — Status bar enxuta (remoção de segmentos redundantes, fusão modelo+pin, acesso preservado via `/jev`, `--doctor`, `/config`, `/mode`)

## Resumo Executivo

Novo pacote `internal/procmon` — um `Manager` dono do ciclo de vida de processos em background, criado no `main.go` e injetado nos três consumidores: a tool `bash` (spawn, via `Registry.SetProcMon`), a TUI (leitura/parada, via `tui.WithProcMon`) e o shutdown (`pm.KillAll()` após `p.Run()`). O manager é puro stdlib (`os/exec`, `syscall`, `sync`, `time`), não importa nada interno e é único por processo — jamais construído dentro de `NewRegistry()`, porque `newSubagent` cria um registry fresco por subagente (agent.go:436) e um manager por registry multiplicaria gerenciadores e geraria órfãos.

Processos em background rodam em seu próprio grupo (`SysProcAttr{Setpgid: true}`) e **nunca** ficam amarrados ao contexto do turno: o branch em background usa `exec.Cmd` puro, sem `CommandContext` e sem o timeout de 120s — caso contrário `esc`/fim de turno mataria silenciosamente processos "de fundo". Detecção de erro de spawn por janela breve de liveness (250ms): processo que morre dentro da janela retorna erro com exit status e stderr capturado (≤2KB) sem criar monitoramento; processo vivo aos 250ms é registrado. Parada individual e shutdown usam o mesmo escalonamento: SIGTERM para o grupo inteiro (`syscall.Kill(-pid)`), SIGKILL após 2s de graça.

A TUI não cacheia estado de processos: `hintBar()` lê `Count()` ao vivo a cada render (como já lê `m.agent.Mode()`/`Pinned()`), e um canal dedicado `Events()` + `procMonMsg` apenas acorda o renderer — espelho do par `listenAgent`/`agentEventMsg` (tui.go:296-307). Eventos de processo não entram no canal do agente: não são eventos de turno, `emit()` descarta quando o buffer de 512 enche e eventos de subagente são roteados como linhas aninhadas — três motivos pelos quais o contador dessincronizaria ou renderizaria errado. A hint bar é reestruturada em lugar (REQ-005): saem `/ commands`, `mode squad`, `resumed · N messages`, `jev` positivo e `pinned X` como segmento próprio; o modelo corrente funde o marcador de pin; entra `bg N` quando há monitoramentos ativos.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/procmon`** (novo) — `Manager`, `Entry`, `proc`; spawn com grupo próprio, watcher que faz `Wait()` exatamente uma vez, drain de pipes (descarta saída, captura ≤2KB só durante a graça de spawn), `Stop` assíncrono com escalonamento TERM→KILL, `KillAll` síncrono e idempotente, canal `Events()` para acordar a TUI. Split por plataforma: `exec_unix.go` (`//go:build unix`) com `launch` (`Setpgid`) e `signalGroup` (`syscall.Kill(-pid, sig)`); `exec_other.go` (`//go:build !unix`) com stubs que retornam erro — a árvore compila em qualquer GOOS, e o kterminal é bash-cêntrico (unix na prática).
- **`internal/tools`** — `Register` ganha guarda de deduplicação de `order` (hoje `Register` appended incondicionalmente, tools.go:55-59; sem a guarda, o re-registro do bash duplicaria o schema enviado ao LLM); `Registry` ganha campo `procMon` + `SetProcMon`/`ProcMon`; `bashTool(pm)` recebe o manager; `runBash` ganha o branch `run_in_background` antes do caminho síncrono, que permanece byte-idêntico.
- **`internal/agent`** — `newSubagent` propaga o manager do registry do pai para o do filho (nil-guardado); `RunSyncPersona` intocado (registry read-only de personas não tem bash — tools.go:47-53); uma linha do `baseSystemPrompt` documenta o parâmetro. O pacote não importa `procmon` — usa o valor retornado por `a.Tools.ProcMon()` sem nomear o tipo.
- **`internal/tui`** — `WithProcMon`, `procMonMsg`/`listenProcMon`, helper nil-guardado `bgCount()`, hint bar reestruturada com `bg N`, popup `/bg` (`bgOpen`/`bgSelected`, primeiro da cadeia de popups), `/bg` em `nativeCommands`, `/help` e `handleCommand`.
- **`main.go`** — cria o manager, injeta via `reg.SetProcMon(pm)` e `tui.WithProcMon(pm)`, chama `pm.KillAll()` nos dois caminhos pós-`p.Run()` (o caminho de erro termina em `os.Exit(1)`, que pula defers — por isso a chamada é explícita em ambos), e instala seguro contra SIGTERM externo.
- **`internal/session`** — sem mudanças. O transcript já registra `tool_call` (args incluem `run_in_background: true`) e `tool_result` (confirmação de lançamento) — esse é o registro audível; saídas naturais e paradas são lifecycle de UI, não de conversação (fora de escopo do PRD: notificação de término).

Fluxo de dados: LLM chama `bash {command, run_in_background: true}` → `Registry.ExecuteStream` → `runBash` → branch background → `procmon.Start(ctx, command)` (ctx usado **apenas** durante a graça) → `Entry{ID, Command, Started}` → `Result{Output: "background process N started: <cmd>"}` → tool_result no transcript e na conversa. Paralelamente o watcher do processo, ao vê-lo morrer, remove do map e acorda a TUI via `Events()` → `procMonMsg` → re-render → `hintBar()` lê `Count()` atualizado.

## Design de Implementação

### Interfaces Principais

`internal/procmon/procmon.go`:

```go
type Entry struct {
    ID      int
    Command string
    Started time.Time
}

type Manager struct {
    mu     sync.Mutex
    next   int
    procs  map[int]*proc
    events chan struct{}
    now    func() time.Time
}

func New() *Manager
func (m *Manager) Events() <-chan struct{}
func (m *Manager) Count() int
func (m *Manager) List() []Entry
func (m *Manager) Start(ctx context.Context, command string) (Entry, error)
func (m *Manager) Stop(id int) bool
func (m *Manager) KillAll()
```

Constantes: `spawnGrace = 250ms`, `killGrace = 2s`, `captureBytes = 2048`, `joinBudget = 2s`.

- **`Start`**: `launch` (bash `-c`, `Setpgid: true`, um `os.Pipe` compartilhado por stdout+stderr; o pai fecha sua ponta de escrita logo após `cmd.Start()`) → watcher em goroutine (`cmd.Wait()` uma única vez, guarda o exit status, `close(done)` — fechado uma única vez —, `m.remove(id)`, `m.notify()`) → drain em goroutine (`io.Copy` para um `discardWriter` que captura ≤2KB apenas enquanto a graça não expirou; EOF quando o processo morre) → `select { <-done → erro de startup; <-timer(spawnGrace) }` → re-check não-bloqueante de `done` (raça na fronteira da janela) → `ctx.Err() != nil` → SIGKILL no grupo e retorna o erro do ctx (esc durante a janela aborta limpo, nada registrado) → registra no map e retorna `Entry`.
- **Erro de startup** (qualquer morte dentro da graça, forma única): `fmt.Errorf("background command exited during startup (exit status %d): %s", code, captured)` — comando inexistente vira exit 127 + "command not found"; comando válido que termina rápido recebe o mesmo tratamento honesto, sem monitoramento fantasma. O buffer de captura é guardado por mutex próprio (o drain escreve concorrentemente; o leitor lê depois de `<-done`, mas o drain pode ainda estar correndo — o mutex fecha a raça).
- **`Stop`**: existe no map? → `signalGroup(pid, SIGTERM)` → `time.AfterFunc(killGrace, …)` re-checka `done` e aplica `signalGroup(pid, SIGKILL)` se o processo resistiu → retorna imediatamente (UX do popup não bloqueia). Sinal para processo já morto → ESRCH, erro ignorado.
- **`KillAll`**: snapshot dos procs sob mutex → TERM em todos → espera bounded por `done` (budget `joinBudget`) → KILL nos sobreviventes → join bounded dos watchers → idempotente (segunda chamada: map vazio, no-op). Garante zero órfãos antes do `main` retornar.
- **Invariantes de concorrência**: `next`, `procs` guardados por `mu`; `done` é fechado só pelo watcher (único dono), leitura concorrente segura; `events` recebe não-bloqueante (wakes coalescem — indiferente, o render é pull-based); exit status escrito pelo watcher antes do `close(done)` (barreira happens-before para quem leu o canal); buffer de captura com mutex próprio; `pid` imutável após o launch.

`internal/tools`:

```go
func (r *Registry) Register(t Tool) {
    if _, ok := r.tools[t.Name]; !ok {
        r.order = append(r.order, t.Name)
    }
    r.tools[t.Name] = t
}

func (r *Registry) SetProcMon(pm *procmon.Manager) {
    r.procMon = pm
    r.Register(bashTool(pm))
}

func (r *Registry) ProcMon() *procmon.Manager
```

`bashTool(pm *procmon.Manager)` — closures chamam `runBash(ctx, args, onLine, pm)`. `NewRegistry()` registra `bashTool(nil)` (síncrono puro); `SetProcMon` re-registra com o manager — a guarda de dedup mantém `Definitions()` com um único bash. `NewReadOnlyRegistry()` intocado (personas não spawnam). Helper novo `optBool(args, key)` (padrão de `optStr`, tools.go:130-137).

Branch em `runBash` (bash.go:44), antes do timeout síncrono:

```go
if optBool(args, "run_in_background") {
    if pm == nil {
        return Result{}, fmt.Errorf("background execution unavailable")
    }
    entry, err := pm.Start(ctx, command)
    if err != nil {
        return Result{}, err
    }
    return Result{Output: fmt.Sprintf("background process %d started: %s", entry.ID, command)}, nil
}
```

O caminho síncrono (`CommandContext`, timeout 120s, merge stdout+stderr, 32KB, lineWriter) permanece intacto — REQ-001 exige zero regressão. Schema ganha a propriedade `"run_in_background": {"type": "boolean", "description": "Set true for long-running processes (dev servers, watchers, long builds): the command returns immediately and keeps running in the background. Output is discarded."}` — o ensino primário vive no schema (sempre enviado via `Definitions()`, inclusive no modo squad, onde o `MaestroPrompt()` substitui o prompt base).

`internal/agent` — propagação em `newSubagent` (agent.go:436), nil-guardada:

```go
if a.Tools != nil {
    sub.Tools.SetProcMon(a.Tools.ProcMon())
}
```

Subagentes lançam no mesmo manager → mesmo `bg N`, mesma lista `/bg`, mesmo kill-all; restringi-los empurraria o LLM para o bash síncrono, que órfã mais (timeout de 120s mata um `npm install` no meio). `prompt.go`: a linha do bash no `baseSystemPrompt` passa a documentar `run_in_background` (processos longos: servidores, watchers, builds; retorna imediato; usuário para via `/bg`).

### Hint Bar (REQ-002 + REQ-005)

`hintBar()` (tui.go:1412) reestruturada em lugar. Lado esquerdo inalterado (spinner+verbo quando busy, cwd quando idle). Lado direito, em ordem:

| # | Segmento | Condição | Estilo |
| --- | --- | --- | --- |
| 1 | `copyFlash` | flash ativo | successStyle |
| 2 | `↑ scrolled · end to jump` | `!follow` | warningStyle |
| 3 | `esc to interrupt` | busy | warningStyle |
| 4 | `skill <name>` | skill ativa | primaryStyle |
| 5 | `bg N` | `bgCount() > 0` | primaryStyle |
| 6 | `jev off` | apenas desvio (Jev desligado/sem chave) | warningStyle |
| 7 | `<model>` ou `⚑ <model>` | modelo corrente; fallback para o pin; `⚑` só com pin ativo | routeStyle |
| 8 | custo | `sessionCost > 0` | routeStyle |
| 9 | `N tok/s` | `lastTPS > 0` | routeStyle |

Removidos: `/ commands`, `resumed · N messages`, `mode squad`, `jev` positivo, `pinned X` como segmento próprio. Fusão modelo+pin: `model := m.currentModel; if model == "" { model = m.agent.Pinned() }`, renderizado uma única vez, com `⚑` prefixado apenas quando `m.agent.Pinned() != ""` — nunca `pinned X` + modelo duplicados. `bg N` lê `bgCount()` ao vivo (nil-safe: manager nil → 0 → segmento oculto); com zero monitoramentos a barra é byte-a-byte a da mesma sessão sem o indicador (critério do PRD). Sessão típica (squad, jev on, sem pin, sem bg) fica com modelo + custo + tok/s — ≤4 segmentos persistentes como pede a métrica de sucesso do PRD. Largura segura: `hintBar` já usa `chatWidth()` (tui.go:1413, 818), que subtrai a sidebar no modo squad, e o enxugamento **reduz** o risco de overflow em larguras estreitas.

### Popup `/bg` (REQ-003)

Espelho do padrão `/mode` (commands.go:105-121): estados `bgOpen`/`bgSelected` no `Model`, helpers `openBgPopup`/`closeBgPopup`/`selectBg`/`bgPopupView`.

- **Abertura**: `/bg` em `handleCommand` (tui.go:1030) → `openBgPopup()` (fecha `cmdPopup`, zera seleção). Com zero monitoramentos: `resultStyle.Render("no background processes running")` no chat, sem popup (critério do PRD).
- **Lista**: `m.procMon.List()` por render (pull-based, sempre fresca) — linhas `#N · <comando truncado> · <uptime>`; comando truncado via `truncate()` existente (rune-safe, testado em tui_test.go:1249) para caber na largura do popup; uptime `time.Since(entry.Started).Round(time.Second)` no formato `5m32s` (estático por render, sem ticker — o PRD lista uptime, não tick ao vivo).
- **Navegação**: `up`/`down` com wrap (idêntico a modeOpen, tui.go:648-680); `enter`/`tab` → `selectBg()`: `Stop(id)` no selecionado, popup **permanece aberto** com a lista atualizada (usuário pode parar vários em sequência); `esc`/`q` fecham sem parar nada. Após stop, se a lista encolheu, `bgSelected` é clampado (`if bgSelected >= len(list) { bgSelected = len(list)-1 }`, e 0 se vazia — vazia mostra o empty-state e enter vira no-op).
- **Posição na cadeia**: bloco `bgOpen` **primeiro** em `handleChatKey`, antes de modeOpen/modelOpen/mentionOpen/cmdOpen — um popup de gerenciamento de processos não deve ser preemptado por sugestões de digitação.
- **Visual**: `chromeLabelStyle.Render("BACKGROUND")` de título, itens com `mentionItemStyle`/`mentionSelectedStyle` (`▸ ` no selecionado), container `mentionPopupStyle`, help line `↑↓ move · enter stop · esc close` — mesmos estilos e glifos dos popups existentes, zero estilos novos em theme.go. Renderizado em `chatView()` junto aos demais popups (tui.go:1632-1643).
- **Edge cases**: terminal estreito — o popup herda o comportamento do mentionPopup (largura do conteúdo, sem clamp explícito); comando mais longo que a largura — truncado pelo `truncate()` da linha; 50+ monitoramentos — lista renderiza completa (o `liveBlockMaxLines` não se aplica; popups existentes não têm cap e o PRD não pede scroll).

### Wiring TUI

```go
type procMonMsg struct{}

func listenProcMon(ch <-chan struct{}) tea.Cmd {
    return func() tea.Msg {
        <-ch
        return procMonMsg{}
    }
}

func WithProcMon(pm *procmon.Manager) Option {
    return func(m *Model) { m.procMon = pm }
}

func (m Model) bgCount() int {
    if m.procMon == nil {
        return 0
    }
    return m.procMon.Count()
}
```

`Init()` (tui.go:296): `return tea.Batch(m.blink, listenAgent(m.agent.Events), listenProcMon(m.procMon.Events()))` — nil-safe porque `WithProcMon` ausente → `m.procMon == nil` → `listenProcMon` recebe canal nil → bloqueia para sempre (comportamento desejado: nunca acorda, nunca re-arma). `Update` ganha o case `procMonMsg`: `return m, listenProcMon(m.procMon.Events())` (re-arma; sem mutação de estado — o render é pull-based, o próximo `View()` já lê `Count()` fresco).

### Shutdown (REQ-004)

`main.go`:

```go
pm := procmon.New()
reg := tools.NewRegistry()
reg.SetProcMon(pm)
...
p := tea.NewProgram(tui.New(ag, cfg, cat, dark, append(opts, tui.WithProcMon(pm))...), ...)
if _, err := p.Run(); err != nil {
    pm.KillAll()
    fmt.Fprintln(os.Stderr, "error:", err)
    sess.Close()
    store.Close()
    os.Exit(1)
}
pm.KillAll()
sess.Close()
```

`/quit`, `/exit` e Ctrl+C (que chega como KeyMsg porque o raw mode desativa ISIG; tui.go:717 `ctrl+c → tea.Quit`) todos convergem em `p.Run()` retornando — um único `KillAll` cobre a letra do REQ-004. O seguro contra `kill` externo: `signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)` antes de `p.Run()`, com goroutine que faz `pm.KillAll(); os.Exit(0)` — as escritas de sessão são appends por linha sem buffer (session.go:80-98), então pular `sess.Close()` não perde nada. (Verificar durante a implementação a API de quit-from-outside do bubbletea v1.3.10; o fallback `KillAll + os.Exit` é seguro e é o especificado.)

## Pontos de Integração

- **Gateway LLM**: intocado. O schema do bash com a nova propriedade é enviado via `Definitions()` como hoje.
- **Jev/roteamento**: intocado. `run_in_background` não afeta `decide()`, pin, fallbacks.
- **Sessão**: intocada. `tool_call`/`tool_result` existentes já registram o lançamento (args + confirmação) — o audit trail do PRD.
- **Squad/personas**: personas continuam sem bash (registry read-only); subagentes compartilham o manager do pai via `SetProcMon` na criação.

## Verificações Técnicas

### Segurança

- Comando arbitrário via `bash -c` — superfície idêntica à tool bash atual; `run_in_background` não amplia (o mesmo comando já era executável de forma síncrona). Confirm mode inalterado por construção: `bash` é `Mutating: true` (bash.go:23), o fluxo `a.Confirm && a.Tools.IsMutating(name)` → `EventConfirm` (agent.go:641-649) aplica-se igualmente; bash não tem `PendingDiff`, então a caixa mostra só o comando (`confirmView` já trata diff nil, tui.go:1597-1613). Decline → "user declined", nenhum processo.
- Sinais para grupo do processo (`-pid`), não para PID individual — mata bash **e** descendentes (`npm run dev` → node), sem tocar processos fora do grupo.
- Nenhuma informação sensível nova em transcript/logs: o comando já era registrado; saída de processo é descartada.

### Arquitetura

- Direção de dependência: `procmon` é folha (stdlib apenas); `tools` importa `procmon`; `agent` **não** importa `procmon` (usa o valor via `a.Tools.ProcMon()` sem nomear o tipo); `tui` e `main` importam `procmon`. Sem ciclos.
- Um manager por processo, injetado — nunca por registry. `newSubagent` cria registry fresco (agent.go:445); a propagação explícita evita managers múltiplos e órfãos de subagente.
- Invariante central: processos em background nunca amarrados ao ctx do turno. O ctx é usado apenas durante a graça de spawn; depois disso, `exec.Cmd` puro vive até o fim natural ou até TERM/KILL.
- Falhas isoladas: erro de spawn → erro da tool, sem monitoramento; manager nil (testes) → branch retorna erro explícito; `Stop` de ID inexistente → `false`, sem efeito.

### Infraestrutura

Binário único, zero dependências novas (stdlib `syscall` para `Setpgid`/`Kill`; `golang.org/x/sys` permanece `// indirect`, go.mod:40 — não promover). Split `//go:build unix` / `!unix` mantém a árvore compilável em qualquer GOOS. Rollback: remover o parâmetro do schema e o branch reverte ao comportamento atual; a dedup em `Register` é inofensiva se órfã.

## Abordagem de Testes

### Testes Unidade

**`internal/procmon/procmon_test.go`** (novo — comandos reais, sem mock de processo):

| Teste | Cenário | Assert |
| --- | --- | --- |
| `TestStartRegistersRunningProcess` | `Start(ctx, "sleep 2")` | retorna em <1s com `Entry.ID > 0`; `Count() == 1`; `List()` tem 1 item com `Command` e `Started` não-zero |
| `TestStartSpawnErrorWithinGrace` | `Start(ctx, "exit 3")` | erro contendo `exit status 3`; `Count() == 0` — nenhum monitoramento fantasma |
| `TestStartUnknownCommand` | `Start(ctx, "definitely-not-a-cmd-xyz")` | erro contendo `127`; `Count() == 0` |
| `TestStartFastLegitCommand` | `Start(ctx, "echo hi")` | erro de startup honesto (terminou na janela), `Count() == 0` |
| `TestStartCtxCanceledDuringGrace` | ctx cancelado após `Start("sleep 30")` | retorna `context.Canceled`; processo morto (grupo); `Count() == 0` |
| `TestNaturalExitDecrementsCount` | `Start("sleep 1")` → espera `Events()` | `Count() == 0` em <5s; `List()` vazia; sem zombie (`Wait()` chamado uma vez) |
| `TestStopKillsProcess` | `Start("sleep 30")` → `Stop(id)` | `Stop` retorna `true` imediato; `Count() == 0` dentro de `killGrace + margem` |
| `TestStopSigtermResistantProcess` | `Start("trap '' TERM; sleep 30")` | morto por SIGKILL dentro de `killGrace`×2; `Count() == 0` |
| `TestStopUnknownID` | `Stop(999)` | retorna `false`, sem pânico, `Count()` inalterado |
| `TestKillAllTerminatesEverything` | 2× `Start("sleep 30")` → `KillAll()` | retorna em <`killGrace`×2; `Count() == 0`; `List()` vazia |
| `TestKillAllIdempotent` | `KillAll()` → `KillAll()` | segunda chamada é no-op, sem pânico/timeout |
| `TestEventsCoalesce` | 3 exits rápidos | `Start`/watcher nunca bloqueiam; leituras do canal coalescem |
| `TestConcurrentStartStop` | N=8 goroutines: metade `Start`, metade `Stop` | sem data race (`-race`), `Count()` consistente no fim |
| `TestChattyChildDoesNotDeadlock` | `Start` de comando que escreve >64KB | `Start` retorna na graça; processo segue vivo (drain funciona) |

**`internal/tools`**:

| Teste | Cenário | Assert |
| --- | --- | --- |
| `TestBashBackgroundReturnsImmediately` (bash_test.go) | `Execute` com `{"command":"sleep 5","run_in_background":true}` | retorna em <1s; output `background process 1 started: sleep 5`; `Count()==1` |
| `TestBashBackgroundSpawnError` | comando `exit 1` em background | erro contendo `exit status 1`; `Count()==0` |
| `TestBashBackgroundNilManager` | registry sem `SetProcMon` | erro `background execution unavailable`; síncrono intacto |
| `TestBashSyncUnchanged` (regressão) | sem `run_in_background` | os testes existentes de bash_test.go (timeout, merge, stream, 32KB) são a prova — todos devem passar sem edição |
| `TestRegisterDedupOrder` (tools_test.go) | `Register` do mesmo nome 2× | `Definitions()` com 1 bash; `order` sem duplicata |
| `TestSetProcMonRewiresBash` | `NewRegistry()` → `SetProcMon(pm)` | `Definitions()` ainda com 1 bash; background funciona |

### Testes de Integração

**`internal/tui`** (padrão `step()`/`newTestModel` existente):

| Teste | Cenário | Assert |
| --- | --- | --- |
| `TestBgBadgeHiddenWhenZero` | `WithProcMon(pm)`, 0 processos | `hintBar()` byte-idêntico ao de model sem a option (critério REQ-002) |
| `TestBgBadgeShowsCount` | `pm.Start("sleep 30")` → render | barra contém `bg 1`; segundo start → `bg 2` |
| `TestBgBadgeLiveDecrement` | `Start("sleep 1")` → `procMonMsg` via Update → render | barra sem `bg` (contador cai sozinho — REQ-002) |
| `TestBgPopupOpenListStop` | 2 starts → `/bg` | popup lista 2; `down` + `enter` → `Count()==1`; popup aberto com lista atualizada; barra `bg 1` |
| `TestBgPopupEscStopsNothing` | 2 starts → `/bg` → `esc` | popup fecha; `Count()==2` |
| `TestBgPopupEmptyState` | `/bg` com 0 | mensagem `no background processes running`; sem popup |
| `TestBgPopupSelectionClamps` | 2 starts → `/bg` → para ambos | seleção clampa; sem pânico com lista vazia |
| `TestBgPopupKeyRouting` | popup aberto → teclas de digitação | não caem no input; setas não movem o cursor do textarea |
| `TestHintBarSlimmedSegments` | sessão squad+jev on+pin+resumed | barra **sem** `/ commands`, `mode squad`, `resumed`, `jev` positivo, `pinned X`; modelo 1× com `⚑` |
| `TestHintBarJevOffStillShown` | jev desligado | `jev off` presente (desvio visível — REQ-005) |
| `TestInitBatchesProcMonListener` | `New(..., WithProcMon(pm))` → `Init()` | cmd batch contém o listener |

**`internal/agent`** (padrão httptest existente):

| Teste | Cenário | Assert |
| --- | --- | --- |
| `TestSubagentSharesProcMon` | agente com `SetProcMon` → `RunSync` que chama bash background | processo registrado no **mesmo** manager do pai |
| `TestBackgroundSurvivesTurnEnd` | turno que lança `sleep 30` em background → turno termina | processo **vivo** após `TurnDone` (invariante central) |
| `TestBackgroundSurvivesEsc` | idem + `esc` durante o turno | processo vivo após `TurnAborted` — ctx do turno não mata background |

O shutdown do `main.go` é coberto pelos testes de `KillAll` no procmon (o `main` não tem harness de teste hoje; o comportamento é do manager).

### Testes de E2E

Não aplicável TestSprite (TUI Go sem frontend web). Verificação E2E das métricas do PRD é manual/por transcript: (1) contador == processos vivos — rodapé contra `ps` com 2 processos ativos; (2) retorno <1s — cronometrar a tool call no transcript; (3) zero órfãos — `ps` após quit com 2 monitoramentos; (4) barra enxuta — sessão típica com ≤4 segmentos.

### Churn de testes existentes (quebra intencional, mapeada)

- `TestHintBarShowsRoutingState` (tui_test.go:952) — reescrever: `pinned glm-5.3` vira `⚑ glm-5.3` no segmento modelo; `jev` positivo vira assert negativo; `jev off` permanece.
- `TestHintBarShowsResumed` (tui_test.go:1296) — **deletar**: o segmento não existe mais (REQ-005).
- `legacyHintBar` (tui_test.go:2618, usada em :2723, testada em :2740) — **atualizar fixture** para a nova barra: o teste guarda regressão do SDD view contra a sidebar, não da hint bar; deletá-lo perderia a guarda.
- `TestCommandPopupListsNativeAndKspec` / `TestCommandPopupWithoutStoreListsNativesOnly` (commands_test.go) — listas exatas ganham `/bg` (ordem alfabética: antes de `/clear`).
- `TestModeCommandSwitchesToSquad` (commands_test.go) — o assert de `mode squad` na barra quebra (removido por REQ-005); trocar para assert negativo (a sidebar continua mostrando o modo).
- `TestHelpListsKspecCommands` — `/help` ganha a linha `/bg — background processes` (assert atualizado).

## Sequenciamento de Desenvolvimento

1. **`internal/procmon`** — pacote completo + testes (fundação isolada; valida spawn/grace/TERM→KILL/kill-all/idempotência sem depender de TUI/agent).
2. **`internal/tools`** — dedup em `Register`, `SetProcMon`/`ProcMon`, `bashTool(pm)`, branch background, `optBool`; testes de unidade + regressão síncrona.
3. **`internal/agent`** — propagação em `newSubagent` + linha do prompt; testes de subagente e de sobrevida a fim de turno/esc.
4. **`internal/tui`** — hint bar reestruturada + `bg N` + churn de fixtures (**junto** com a barra, ou `TestSDDViewByteIdenticalToPreSidebar` falha enganosamente); depois `WithProcMon`/`listenProcMon`/popup `/bg`/`/help`/`nativeCommands` + testes.
5. **`main.go`** — wiring + `KillAll` nos dois caminhos + seguro de sinal.
6. **Verificação final** — `go build ./... && go vet ./... && gofmt -l . && go test ./... -race` (o `-race` é essencial: procmon é o primeiro código concorrente de processo no repo) + checagem manual das métricas do PRD.

### Dependências Técnicas

Nenhuma bloqueante. Verificar no passo 5 a API de quit-from-outside do bubbletea v1.3.10 (fallback seguro especificado: `KillAll + os.Exit`). `syscall.SysProcAttr.Setpgid` e `syscall.Kill(-pid)` existem no stdlib darwin/linux — confirmar no CI alvo.

## Monitoramento e Observabilidade

### Error Tracking

CLI local, sem Sentry. Erros de spawn → erro da tool (visível na conversa + transcript `tool_result` com `error:`). Erros de sinal (ESRCH em processo já morto) → ignorados por design. `KillAll` não retorna erro — sobreviventes após SIGKILL são improváveis; se o `joinBudget` estourar, o `main` segue o exit e o SO limpa no reaping quando o pai morre (último recurso).

### Logging Estruturado

O transcript JSONL existente é a fonte de verdade: `tool_call` com `"run_in_background":true` nos args + `tool_result` com `background process N started: <cmd>` — par lançamento/ID auditável por sessão. Nada de lifecycle de UI (exits/paradas) no transcript — decisão do PRD (fora de escopo: notificação de término). Sem dados sensíveis novos.

### Health Checks

`--doctor` intocado. O `bg N` na barra é o health check contínuo do usuário; `/bg` é a inspeção detalhada (comando + uptime por processo).

### Métricas de Negócio

Métricas do PRD verificáveis por teste (contador == vivos; retorno <1s; zero órfãos pós-quit; ≤4 segmentos) — cobertas nos planos de unidade/integração acima. Telemetria de TPS por modelo intocada.

### Alertas

Não aplicável (CLI local sem canal de notificação).

## Considerações Técnicas

### Decisões Principais

1. **Pacote novo `internal/procmon`, manager criado no `main.go`** — três consumidores em três camadas (tools spawn, tui read/stop, main kill-all); um manager dentro de `internal/tools` forçaria lifecycle de processo num pacote de dispatch e a TUI alcançaria dentro dele para leituras de UI. Decisivo: `newSubagent` cria registry fresco (agent.go:445) — manager por registry multiplicaria gerenciadores e geraria órfãos de subagente. Alternativa rejeitada: manager no `Registry`.
2. **Pull-based + canal de wake dedicado** — `hintBar()` lê `Count()` por render (como já lê `Mode()`/`Pinned()`); `procMonMsg`/`listenProcMon` apenas re-renderizam. Eventos de processo **não** entram no canal do agente: `emit()` descarta com buffer cheio (agent.go:302-307) → contador dessincronizaria; eventos Depth>0 renderizam como aninhados (tui.go:470-497); e viraria um canal de turno num bus geral. Alternativa rejeitada: polling com ticker.
3. **Janela de liveness de 250ms para erro de spawn** — `LookPath` é estruturalmente errado (não vê builtins, pipes, `&&`, funções de shell). "Retorna em <1s" comporta 250ms folgado; `bash -c "cmd-inexistente"` sai 127 em ~10ms; vivo aos 250ms é um long-runner real. Comando que termina legitimamente na janela recebe "exited during startup (status N)" — honesto para fire-and-forget. Alternativa rejeitada: `LookPath` (falso-negativo em comandos legais).
4. **Grupo de processo via stdlib puro** — `SysProcAttr{Setpgid: true}` + `syscall.Kill(-pid, SIGTERM)` mata bash e descendentes; ambos no stdlib darwin/linux. `golang.org/x/sys` ficaria `// indirect` → direto — desnecessário. Split `//go:build unix` cobre `Setpgid` **e** `signalGroup` (ambos unix-only).
5. **`KillAll` após `p.Run()` + seguro de sinal** — quit normal e Ctrl+C convergem em `Run()` retornando; `kill` externo terminaria o processo sem cleanup Go → `signal.Notify` roteia para `KillAll + os.Exit` (escritas de sessão são appends não-buferados, session.go:80-98 — nada se perde pulando `Close()`).
6. **Subagentes compartilham o manager** — restringi-los empurraria o LLM para bash síncrono, que órfã mais (timeout de 120s mata `npm install` no meio). Compartilhar = mesmo `bg N`, mesma lista, mesmo kill-all. Personas não spawnam (registry read-only sem bash) — intocado.
7. **Fixture `legacyHintBar` atualizada, não deletada** — o teste byte-idêntico guarda o SDD view contra regressão da **sidebar**; seu propósito não é a hint bar. Atualizar o fixture mantém a guarda honesta.
8. **Sem writes novos de transcript** — `tool_call`/`tool_result` existentes são o registro; lifecycle de UI no transcript vazaria ruído em `messages` no resume (snapshots alimentam o contexto do LLM).

### Riscos Conhecidos

- **Acoplamento ao ctx do turno (risco máximo)**: se o branch background usar `CommandContext`/ctx do turno, `esc`/fim de turno mata processos "de fundo". Mitigação: `exec.Cmd` puro, sem wrap de timeout; testes dedicados (`TestBackgroundSurvivesTurnEnd`/`Esc`).
- **Deadlock de pipe**: pipes não drenados bloqueiam filho tagarela em 64KB. Mitigação: goroutine de drain permanente; teste com processo escrevendo >64KB.
- **Duplicação de `Register`**: sem a guarda, `SetProcMon` enviaria o schema do bash 2× ao LLM. Mitigação: guarda de dedup + teste `TestRegisterDedupOrder`.
- **Raça na fronteira da graça**: `select` com `done` e timer prontos pode registrar processo recém-morto. Mitigação: re-check não-bloqueante de `done` antes de registrar; pior caso é um blip de um frame no contador.
- **Caminhos nil-manager**: testes constroem agents/TUIs com registry nil (tui_test.go:32-35) e options ausentes — toda leitura nova é nil-guardada (`bgCount()`; guarda em `newSubagent` sobre `a.Tools`).
- **Incerteza de API de quit-from-outside** no bubbletea v1.3.10: verificar na implementação; o fallback `KillAll + os.Exit` é seguro e especificado.
- **Lockstep de fixtures**: `legacyHintBar` deve mudar no mesmo commit da `hintBar`, ou o teste byte-idêntico falha enganosamente.
- **Higiene de zombies**: watchers devem chamar `Wait()` exatamente uma vez e o `KillAll` deve dar join bounded neles, ou `Count()` deriva; inofensivo ao SO, visível no `bg N`.
- **Uptime estático no `/bg`**: renderiza por abertura/keypress, sem ticker (o PRD lista uptime, não tick ao vivo); revisitar se o QA sinalizar.

### Conformidade com Skills Padrões

As rules de `.agents/rules/` são orientadas ao ecossistema kspec TS/Java; para o código Go prevalecem os padrões do repo (AGENTS.md): sem comentários no código, eventos do agente via canal, receivers por valor na TUI, textos de TUI em inglês, docs em pt-BR, sem dependências novas. As rules de testes TS (TestSprite) não se aplicam — testes seguem o padrão `go test` + httptest do código-base.

### Arquivos relevantes e dependentes

- `internal/procmon/procmon.go` — `Manager`/`Entry`/`proc`, spawn/grace/stop/kill-all (novo)
- `internal/procmon/exec_unix.go` — `launch` com `Setpgid`, `signalGroup` via `syscall.Kill(-pid)` (novo, `//go:build unix`)
- `internal/procmon/exec_other.go` — stubs que retornam erro (novo, `//go:build !unix`)
- `internal/procmon/procmon_test.go` — testes de unidade (novo)
- `internal/tools/tools.go` — dedup em `Register`, `procMon`/`SetProcMon`/`ProcMon` (modificado)
- `internal/tools/bash.go` — `bashTool(pm)`, branch `run_in_background`, `optBool`, schema (modificado)
- `internal/tools/bash_test.go`, `internal/tools/tools_test.go` — testes novos + regressão (modificados)
- `internal/agent/agent.go` — propagação em `newSubagent` (modificado)
- `internal/agent/prompt.go` — linha do bash no `baseSystemPrompt` (modificado)
- `internal/agent/agent_test.go` — subagente/sobrevida (modificado)
- `internal/tui/tui.go` — `procMon`/`WithProcMon`/`bgCount`/`procMonMsg`/`listenProcMon`, `hintBar()`, `handleChatKey`, `handleCommand`, `/help`, `chatView` (modificado)
- `internal/tui/commands.go` — `/bg` em `nativeCommands`, `openBgPopup`/`closeBgPopup`/`selectBg`/`bgPopupView` (modificado)
- `internal/tui/theme.go` — sem mudanças (estilos existentes reutilizados)
- `internal/tui/tui_test.go` — `TestHintBarShowsRoutingState` reescrito, `TestHintBarShowsResumed` deletado, `legacyHintBar` atualizada, testes novos de badge/popup (modificado)
- `internal/tui/commands_test.go` — listas de popup com `/bg`, asserts de hint bar atualizados (modificado)
- `main.go` — `procmon.New()`, `SetProcMon`, `WithProcMon`, `KillAll` nos dois caminhos, seguro de sinal (modificado)
- `internal/session/session.go` — sem mudanças (transcript existente é o registro)
- `spec/tasks/016-prd-monitoramento-background/prd.md` — fonte dos requisitos





