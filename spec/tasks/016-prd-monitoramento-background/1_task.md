# Tarefa 1.0: Pacote `internal/procmon` — Manager de processos em background

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Lançamento em background via tool `bash` (fundação: spawn com grupo próprio, graça de spawn, erro de spawn sem monitoramento fantasma)
- REQ-003 — Seletor interativo para parar monitoramentos (fundação: `Stop` com escalonamento SIGTERM → SIGKILL, `List` com ID/comando/uptime)
- REQ-004 — Encerramento na saída do kterminal (fundação: `KillAll` síncrono e idempotente)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Criar o pacote `internal/procmon` — um `Manager` dono do ciclo de vida de processos em background. É a fundação isolada da feature: puro stdlib (`os/exec`, `syscall`, `sync`, `time`), não importa nada interno do repo, e valida spawn/graça/TERM→KILL/kill-all/idempotência sem depender de TUI/agent. O manager é **único por processo** e será criado no `main.go` (task 7.0) — jamais dentro de `NewRegistry()`, porque `newSubagent` cria um registry fresco por subagente (agent.go:436) e um manager por registry multiplicaria gerenciadores e geraria órfãos.

Processos em background rodam em seu próprio grupo (`SysProcAttr{Setpgid: true}`) e **nunca** ficam amarrados ao contexto do turno: o `Start` usa `exec.Cmd` puro, sem `CommandContext` e sem timeout — o ctx recebido é usado **apenas** durante a janela de graça de spawn. Detecção de erro de spawn por janela breve de liveness (250ms): processo que morre dentro da janela retorna erro com exit status e stderr capturado (≤2KB) sem criar monitoramento; processo vivo aos 250ms é registrado.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários no código, código em inglês, stdlib apenas (zero dependências novas — `golang.org/x/sys` permanece `// indirect`), split `//go:build unix`/`!unix` mantém a árvore compilável em qualquer GOOS.
</skills>

<requirements>
- Assinaturas exatamente como na seção "Interfaces Principais" da techspec.md: `Entry{ID int; Command string; Started time.Time}`, `Manager{mu sync.Mutex; next int; procs map[int]*proc; events chan struct{}; now func() time.Time}`, `New()`, `Events()`, `Count()`, `List()`, `Start(ctx, command)`, `Stop(id)`, `KillAll()`
- Constantes: `spawnGrace = 250ms`, `killGrace = 2s`, `captureBytes = 2048`, `joinBudget = 2s`
- `Start`: `launch` (bash `-c`, `Setpgid: true`, um `os.Pipe` compartilhado por stdout+stderr; pai fecha sua ponta de escrita logo após `cmd.Start()`) → watcher em goroutine (`cmd.Wait()` uma única vez, guarda exit status, `close(done)` uma única vez, `m.remove(id)`, `m.notify()`) → drain em goroutine (`io.Copy` para `discardWriter` que captura ≤2KB apenas enquanto a graça não expirou) → `select { <-done → erro de startup; <-timer(spawnGrace) }` → re-check não-bloqueante de `done` (raça na fronteira da janela) → `ctx.Err() != nil` → SIGKILL no grupo e retorna erro do ctx → registra no map e retorna `Entry`
- Erro de startup (qualquer morte dentro da graça, forma única): `fmt.Errorf("background command exited during startup (exit status %d): %s", code, captured)` — comando inexistente vira exit 127 + "command not found"
- Buffer de captura guardado por mutex próprio (drain escreve concorrentemente; leitor lê após `<-done`)
- `Stop`: existe no map? → `signalGroup(pid, SIGTERM)` → `time.AfterFunc(killGrace, …)` re-checka `done` e aplica SIGKILL se resistiu → retorna imediatamente (UX do popup não bloqueia); ESRCH em processo já morto → ignorado
- `KillAll`: snapshot dos procs sob mutex → TERM em todos → espera bounded por `done` (budget `joinBudget`) → KILL nos sobreviventes → join bounded dos watchers → idempotente (segunda chamada: map vazio, no-op)
- Invariantes de concorrência da techspec: `next`/`procs` sob `mu`; `done` fechado só pelo watcher; exit status escrito antes do `close(done)`; `events` recebe não-bloqueante (wakes coalescem); `pid` imutável após launch
- Split por plataforma: `exec_unix.go` (`//go:build unix`) com `launch` (`Setpgid`) e `signalGroup` (`syscall.Kill(-pid, sig)`); `exec_other.go` (`//go:build !unix`) com stubs que retornam erro
</requirements>

## Subtarefas

- [ ] 1.1 Criar `internal/procmon/procmon.go` com `Manager`/`Entry`/`proc`, constantes (`spawnGrace`, `killGrace`, `captureBytes`, `joinBudget`), `New()`, `Events()`, `Count()`, `List()` e o `discardWriter` com captura limitada por mutex
- [ ] 1.2 Implementar `Start`: launch com pipe compartilhado stdout+stderr, watcher (`Wait()` único + `close(done)` + remove + notify), drain permanente, select graça/done com re-check não-bloqueante, cancelamento de ctx durante a graça (SIGKILL no grupo, nada registrado)
- [ ] 1.3 Implementar `Stop` assíncrono com escalonamento TERM → KILL via `time.AfterFunc` e `KillAll` síncrono idempotente com join bounded
- [ ] 1.4 Criar `exec_unix.go` (`launch` com `Setpgid: true`, `signalGroup` via `syscall.Kill(-pid, sig)`) e `exec_other.go` (stubs com erro)
- [ ] 1.5 Escrever `procmon_test.go` com os 14 testes da tabela "Testes Unidade" da techspec (comandos reais, sem mock de processo), rodando com `-race`

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" (bloco `internal/procmon` completo com o passo-a-passo de `Start`/`Stop`/`KillAll` e invariantes de concorrência), "Arquitetura do Sistema" (componente `internal/procmon`) e "Riscos Conhecidos" (deadlock de pipe, raça na fronteira da graça, higiene de zombies). A tabela completa de testes de unidade está em "Abordagem de Testes → Testes Unidade".

## Critérios de Sucesso

- `go build ./...` verde em unix (e o split `!unix` compila — `GOOS=windows go build ./internal/procmon/`)
- `go test ./internal/procmon/ -race` verde: `Start` retorna em <1s com processo vivo; morte na graça → erro com exit status e `Count()==0`; `Stop` mata dentro de `killGrace`; resistente a TERM morre por KILL; `KillAll` idempotente; processo tagarela (>64KB) não deadlocka; N=8 goroutines sem data race
- `Events()` acorda leitor após morte natural sem bloquear o watcher
- Zero dependências novas em go.mod

## Testes da Tarefa

- [ ] Testes de unidade (`internal/procmon/procmon_test.go`, 14 testes conforme tabela da techspec):
  - [ ] `TestStartRegistersRunningProcess` — `sleep 2` retorna <1s, `Count()==1`, `List()` com Command/Started
  - [ ] `TestStartSpawnErrorWithinGrace` — `exit 3` → erro com `exit status 3`, `Count()==0`
  - [ ] `TestStartUnknownCommand` — comando inexistente → erro com `127`, `Count()==0`
  - [ ] `TestStartFastLegitCommand` — `echo hi` → erro de startup honesto, `Count()==0`
  - [ ] `TestStartCtxCanceledDuringGrace` — ctx cancelado → `context.Canceled`, processo morto, `Count()==0`
  - [ ] `TestNaturalExitDecrementsCount` — `sleep 1` → `Events()` acorda, `Count()==0` em <5s, sem zombie
  - [ ] `TestStopKillsProcess` — `Stop` retorna `true` imediato, `Count()==0` dentro de `killGrace + margem`
  - [ ] `TestStopSigtermResistantProcess` — `trap '' TERM` → morto por SIGKILL dentro de `killGrace`×2
  - [ ] `TestStopUnknownID` — `Stop(999)` → `false`, sem pânico
  - [ ] `TestKillAllTerminatesEverything` — 2 processos → `Count()==0`, `List()` vazia
  - [ ] `TestKillAllIdempotent` — segunda chamada é no-op
  - [ ] `TestEventsCoalesce` — 3 exits rápidos, sem bloqueio
  - [ ] `TestConcurrentStartStop` — N=8 goroutines, sem data race com `-race`
  - [ ] `TestChattyChildDoesNotDeadlock` — >64KB de saída, `Start` retorna na graça, processo vivo
- [ ] Testes de integração: N/A (pacote folha isolado; integração acontece nas tasks 2.0, 5.0 e 7.0)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/procmon/procmon.go` (novo)
- `internal/procmon/exec_unix.go` (novo, `//go:build unix`)
- `internal/procmon/exec_other.go` (novo, `//go:build !unix`)
- `internal/procmon/procmon_test.go` (novo)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
