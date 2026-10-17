# Tarefa 7.0: main.go — wiring dos três consumidores, `KillAll` nos dois caminhos e seguro de sinal externo

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Encerramento de todos os monitoramentos na saída do kterminal (zero órfãos)

## Dependências

- 2.0 (injeção via `reg.SetProcMon(pm)`)
- 3.0 (propagação de subagentes depende do manager presente no registry do pai)
- 5.0 (injeção via `tui.WithProcMon(pm)`)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: < 2h

## Visão Geral

Fechar o ciclo de vida no `main.go`: criar o manager único, injetá-lo nos três consumidores (tool `bash` via `reg.SetProcMon`, TUI via `tui.WithProcMon`, shutdown via `pm.KillAll()`) e garantir zero órfãos nos dois caminhos pós-`p.Run()` — o caminho de erro termina em `os.Exit(1)`, que pula defers, por isso a chamada é explícita em ambos. `/quit`, `/exit` e Ctrl+C (que chega como KeyMsg porque o raw mode desativa ISIG; tui.go:717 `ctrl+c → tea.Quit`) todos convergem em `p.Run()` retornando — um único `KillAll` cobre a letra do REQ-004.

Adicionalmente, o seguro contra `kill` externo: `signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)` antes de `p.Run()`, com goroutine que faz `pm.KillAll(); os.Exit(0)` — as escritas de sessão são appends por linha sem buffer (session.go:80-98), então pular `sess.Close()` não perde nada.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, wiring no `main.go` como os demais componentes (agent, config, session), stdlib apenas.
</skills>

<requirements>
- Criar o manager no `main.go`: `pm := procmon.New()` **antes** da construção do registry; `reg.SetProcMon(pm)` logo após `tools.NewRegistry()`
- `tui.WithProcMon(pm)` no slice de options passado a `tui.New`
- `pm.KillAll()` explícito nos dois caminhos pós-`p.Run()`: caminho de erro (antes do `os.Exit(1)`) e caminho normal (antes de `sess.Close()`)
- Seguro de sinal: `signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)` antes de `p.Run()`; goroutine que faz `pm.KillAll(); os.Exit(0)` ao receber
- Verificar durante a implementação a API de quit-from-outside do bubbletea v1.3.10; o fallback `KillAll + os.Exit` é seguro e é o especificado
- Manager único por processo — nunca construído dentro de `NewRegistry()` (newSubagent cria registry fresco por subagente, agent.go:436)
</requirements>

## Subtarefas

- [ ] 7.1 Criar `pm := procmon.New()` e injetar via `reg.SetProcMon(pm)` e `tui.WithProcMon(pm)` nas options
- [ ] 7.2 Adicionar `pm.KillAll()` explícito nos dois caminhos pós-`p.Run()` (erro e normal)
- [ ] 7.3 Instalar o seguro de sinal SIGTERM/SIGINT externo com goroutine `KillAll + os.Exit(0)`; verificar a API de quit-from-outside do bubbletea v1.3.10
- [ ] 7.4 Verificar compilação e suite existente (`go build ./... && go vet ./... && go test ./... -race`)

## Detalhes de Implementação

Consulte techspec.md — seções "Shutdown (REQ-004)" (bloco de código completo do `main.go` com os dois caminhos e o seguro de sinal), "Arquitetura do Sistema" (componente `main.go`) e "Dependências Técnicas" (verificação da API do bubbletea v1.3.10). Racional em "Considerações Técnicas → Decisões Principais" (decisão 5).

## Critérios de Sucesso

- `go build ./... && go vet ./...` verdes; `go test ./... -race` verde
- Sair com 2 monitoramentos ativos → nenhum dos dois processos permanece vivo após o encerramento (verificável com `ps` — métrica do PRD)
- Processo resistente a SIGTERM é morto por SIGKILL (comportamento do `KillAll`, task 1.0, agora no caminho real de shutdown)
- Caminho de erro do `p.Run()` também mata tudo (KillAll antes do `os.Exit(1)`)
- `kill` externo (SIGTERM) dispara `KillAll` antes do exit
- Binário único, zero dependências novas

## Testes da Tarefa

- [ ] Testes de unidade: N/A (o `main` não tem harness de teste hoje; o comportamento de `KillAll` é coberto pelos testes do procmon na task 1.0 — `TestKillAllTerminatesEverything`, `TestKillAllIdempotent`, `TestStopSigtermResistantProcess`)
- [ ] Testes de integração: N/A (wiring puro; os três consumidores são testados nas tasks 2.0, 3.0 e 5.0)
- [ ] Testes E2E (verificação manual das métricas do PRD, por transcript/`ps`):
  - [ ] Zero órfãos: subir 2 monitoramentos, `/quit`, `ps` mostra nenhum processo sobrevivente
  - [ ] Ctrl+C com monitoramentos ativos → mesmo resultado
  - [ ] `kill -TERM <pid do kterminal>` → processos encerrados antes do exit

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go` (modificado — `procmon.New()`, `SetProcMon`, `WithProcMon`, `KillAll` nos dois caminhos, seguro de sinal)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
