# Tarefa 2.0: Tool `bash` com `run_in_background` — dedup em `Register`, `SetProcMon` e branch background

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Lançamento em background via tool `bash` (parâmetro `run_in_background`, retorno imediato, erro de spawn, múltiplos monitoramentos, saída descartada, schema)

## Dependências

- 1.0 (o branch background consome `procmon.Manager`)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Estender `internal/tools` para que a tool `bash` existente ganhe o parâmetro booleano `run_in_background`. `Registry` ganha campo `procMon` + `SetProcMon`/`ProcMon`; `bashTool(pm)` passa a receber o manager e `runBash` ganha o branch background **antes** do caminho síncrono, que permanece byte-idêntico (REQ-001 exige zero regressão). `NewRegistry()` registra `bashTool(nil)` (síncrono puro); `SetProcMon` re-registra com o manager — e a guarda de deduplicação em `Register` é o que impede o schema do bash de ser enviado 2× ao LLM (hoje `Register` appended `order` incondicionalmente, tools.go:55-59).

O ensino primário do parâmetro vive no schema (sempre enviado via `Definitions()`, inclusive no modo squad, onde o `MaestroPrompt()` substitui o prompt base).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, tools registradas no construtor do `Registry`, helper `optBool` segue o padrão de `optStr` (tools.go:130-137).
</skills>

<requirements>
- `Register` com guarda de deduplicação: `if _, ok := r.tools[t.Name]; !ok { r.order = append(r.order, t.Name) }` antes do assignment
- `Registry` ganha campo `procMon *procmon.Manager` + `SetProcMon(pm)` (que re-registra `bashTool(pm)`) + `ProcMon()`
- `bashTool(pm *procmon.Manager)` — closures chamam `runBash(ctx, args, onLine, pm)`; `NewRegistry()` registra `bashTool(nil)`; `NewReadOnlyRegistry()` intocado (personas não spawnam)
- Helper novo `optBool(args, key)` no padrão de `optStr` (tools.go:130-137)
- Branch background em `runBash` (bash.go:44), **antes** do timeout síncrono, exatamente como na techspec: `optBool(args, "run_in_background")` → pm nil → erro `background execution unavailable`; senão `pm.Start(ctx, command)` → erro → retorna erro; sucesso → `Result{Output: fmt.Sprintf("background process %d started: %s", entry.ID, command)}`
- O caminho síncrono (`CommandContext`, timeout 120s, merge stdout+stderr, 32KB, lineWriter) permanece intacto
- Schema ganha a propriedade `"run_in_background": {"type": "boolean", "description": "Set true for long-running processes (dev servers, watchers, long builds): the command returns immediately and keeps running in the background. Output is discarded."}`
- Confirm mode inalterado por construção: `bash` permanece `Mutating: true` (bash.go:23)
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar guarda de dedup em `Register` (tools.go:55-59) e campo `procMon` + `SetProcMon`/`ProcMon` no `Registry`
- [ ] 2.2 Converter `bashTool()` em `bashTool(pm *procmon.Manager)` com closures repassando pm; `NewRegistry()` passa `nil`; adicionar `optBool` no padrão de `optStr`
- [ ] 2.3 Adicionar branch `run_in_background` em `runBash` antes do caminho síncrono e a propriedade booleana no schema
- [ ] 2.4 Testes: `TestBashBackgroundReturnsImmediately`, `TestBashBackgroundSpawnError`, `TestBashBackgroundNilManager` em bash_test.go; `TestRegisterDedupOrder`, `TestSetProcMonRewiresBash` em tools_test.go; regressão síncrona (testes existentes passam sem edição)

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" (bloco `internal/tools` com o código de `Register`/`SetProcMon`/`bashTool` e o branch em `runBash`), "Pontos de Integração" (Gateway LLM: schema via `Definitions()`) e "Riscos Conhecidos" (duplicação de `Register`). Tabela de testes em "Abordagem de Testes → Testes Unidade → `internal/tools`".

## Critérios de Sucesso

- `go test ./internal/tools/ -race` verde
- `Execute` com `{"command":"sleep 5","run_in_background":true}` retorna em <1s com output `background process 1 started: sleep 5` e `Count()==1`
- Comando `exit 1` em background → erro contendo `exit status 1`, `Count()==0`
- Registry sem `SetProcMon` → erro `background execution unavailable`; caminho síncrono intacto
- `NewRegistry()` → `SetProcMon(pm)` → `Definitions()` com exatamente 1 bash
- Todos os testes existentes de bash_test.go (timeout, merge, stream, 32KB) passam **sem edição**

## Testes da Tarefa

- [ ] Testes de unidade:
  - [ ] `TestBashBackgroundReturnsImmediately` (bash_test.go) — retorno <1s, output correto, `Count()==1`
  - [ ] `TestBashBackgroundSpawnError` — erro com `exit status 1`, `Count()==0`
  - [ ] `TestBashBackgroundNilManager` — erro `background execution unavailable`, síncrono intacto
  - [ ] `TestRegisterDedupOrder` (tools_test.go) — mesmo nome 2× → 1 bash em `Definitions()`, `order` sem duplicata
  - [ ] `TestSetProcMonRewiresBash` — re-registro mantém 1 bash, background funciona
  - [ ] Regressão: testes existentes de bash_test.go passam sem edição (prova do REQ-001 "comportamento síncrono intacto")
- [ ] Testes de integração: N/A (a integração com agent/TUI acontece nas tasks 3.0, 5.0 e 7.0)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` (modificado — dedup em `Register`, `procMon`/`SetProcMon`/`ProcMon`)
- `internal/tools/bash.go` (modificado — `bashTool(pm)`, branch `run_in_background`, `optBool`, schema)
- `internal/tools/bash_test.go` (modificado — testes novos + regressão)
- `internal/tools/tools_test.go` (modificado — dedup e rewire)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
