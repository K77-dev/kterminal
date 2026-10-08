# Tarefa 3.0: Flags CLI `--continue`/`--session` e wiring de resume

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Flags CLI

## Dependências

- 2.0 (`SetMessages` no agent; snapshot sendo gravado a cada turno)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

O ponto de entrada do resume para o usuário: `main.go` ganha as flags `--continue` (carrega a sessão mais recente) e `--session <caminho>` (carrega o arquivo indicado). O wiring é: load → `agent.SetMessages(messages)` → `session.AppendWriter(pathOriginal)` (mesmo arquivo, append) → sinalizar a TUI com as mensagens para reconstrução visual (task 4.0). Erros seguem o contrato do PRD: `--session` com arquivo inexistente/sem snapshot → erro claro no stderr e exit 1; `--continue` sem sessões → aviso no chat ("no previous session — starting fresh") e app funcional. `--session` vence sobre `--continue` quando ambos usados (documentado no help).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`flag`, `fmt`, `os`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (fluxo conversar→sair→retomar), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; `main → session/agent/tui` (direção de dependência).
</skills>

<requirements>
- Flag `--continue`: `session.LoadLatest()`; sucesso → `SetMessages` + `AppendWriter(path)` + sinalizar TUI; `ErrNoSessions` → aviso "no previous session — starting fresh" no chat e app segue fresh.
- Flag `--session <caminho>`: `session.Load(path)`; erro (inexistente/sem snapshot/corrompido) → `fmt.Fprintln(os.Stderr, "kterminal:", err)` + `os.Exit(1)`.
- `--continue` e `--session` juntos: `--session` vence (mais específico); documentado no help.
- A sessão retomada grava no **mesmo arquivo** JSONL original (append).
- Erros de resume são explícitos e acionáveis — nunca silenciosos, nunca crash.
</requirements>

## Subtarefas

- [ ] 3.1 Declarar as flags `--continue` e `--session` no `main.go`
- [ ] 3.2 Implementar o wiring de `--continue` (LoadLatest → SetMessages → AppendWriter → sinalizar TUI; aviso quando vazio)
- [ ] 3.3 Implementar o wiring de `--session` (Load → SetMessages → AppendWriter; stderr + exit 1 no erro)
- [ ] 3.4 Documentar a precedência `--session` > `--continue` no help das flags

## Detalhes de Implementação

- Snippet completo do wiring no `main.go`: techspec, seção **Design de Implementação → Interfaces Principais** (bloco de código `main.go`).
- Regras de erro/aviso e precedência: techspec, seção **Design de Implementação** (itens após o snippet) e PRD, seção **REQ-003**.
- Error tracking (stderr + exit 1 vs aviso no chat): techspec, seção **Monitoramento e Observabilidade → Error Tracking**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Fluxo manual: conversar, sair, `kterminal --continue` → a sessão carrega (validação completa do fluxo é E2E da task 5.0/QA; aqui o wiring compila e o caminho feliz executa sem pânico).
- `--session` com caminho inexistente → mensagem clara no stderr e exit 1.
- `--continue` sem sessões → app inicia normalmente (aviso configurado para a TUI).

## Testes da Tarefa

- [ ] Testes de unidade — o wiring do `main.go` é fino e validado em E2E (techspec, seção **Abordagem de Testes → Testes de Integração**); os contratos de `Load`/`LoadLatest`/`AppendWriter` estão cobertos na task 1.0.
- [ ] Testes de integração — cobertos pelos testes de session/agent (contrato ponta a ponta, conforme techspec).
- [ ] Testes E2E — deferidos para `kspec-qa`: conversar → sair → `--continue` → pergunta nova responde com contexto; `--session` inexistente → stderr + exit 1; `--continue` sem sessões → aviso no chat e app funcional.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go` — flags `--continue`/`--session`, wiring, exit codes
- `internal/session/session.go` — `Load`/`LoadLatest`/`AppendWriter` consumidos (task 1.0)
- `internal/agent/agent.go` — `SetMessages` consumido (task 2.0)
