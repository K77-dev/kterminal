# Tarefa 3.0: Agente — propagação do manager para subagentes e documentação no prompt

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Lançamento em background via tool `bash` (subagentes compartilham o manager do pai; prompt do sistema instrui o LLM sobre quando usar `run_in_background`)

## Dependências

- 2.0 (usa `Registry.ProcMon()` e o bash com branch background)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Propagar o manager do registry do pai para o do filho em `newSubagent` (agent.go:436), nil-guardado, e documentar o parâmetro `run_in_background` na linha do bash do `baseSystemPrompt`. Subagentes que lançam no mesmo manager do pai significam mesmo `bg N`, mesma lista `/bg` e mesmo kill-all — restringi-los empurraria o LLM para o bash síncrono, que órfã mais (timeout de 120s mata um `npm install` no meio). O pacote `agent` **não** importa `procmon`: usa o valor retornado por `a.Tools.ProcMon()` sem nomear o tipo.

Esta task também carrega os testes do invariante central da feature: processos em background **nunca** amarrados ao ctx do turno — se o branch usasse `CommandContext`/ctx do turno, `esc`/fim de turno mataria silenciosamente processos "de fundo" (risco máximo da techspec, mitigado na task 1.0 e provado aqui).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, eventos do agente via canal, testes de agent seguem o padrão httptest existente.
</skills>

<requirements>
- Propagação em `newSubagent` (agent.go:436), nil-guardada: `if a.Tools != nil { sub.Tools.SetProcMon(a.Tools.ProcMon()) }` — `SetProcMon` com nil é seguro (bashTool(nil) = síncrono puro)
- `RunSyncPersona` intocado (registry read-only de personas não tem bash, tools.go:47-53)
- `prompt.go`: a linha do bash no `baseSystemPrompt` passa a documentar `run_in_background` (processos longos: servidores, watchers, builds; retorna imediato; usuário para via `/bg`)
- O pacote `agent` não importa `procmon` — direção de dependência da techspec (usa o valor via `a.Tools.ProcMon()` sem nomear o tipo)
- Testes com padrão httptest existente de agent_test.go
</requirements>

## Subtarefas

- [ ] 3.1 Adicionar a propagação nil-guardada em `newSubagent` (agent.go:436)
- [ ] 3.2 Atualizar a linha do bash no `baseSystemPrompt` (prompt.go) documentando `run_in_background`
- [ ] 3.3 Testes em agent_test.go: `TestSubagentSharesProcMon`, `TestBackgroundSurvivesTurnEnd`, `TestBackgroundSurvivesEsc`

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" (bloco `internal/agent` com o snippet da propagação), "Arquitetura do Sistema" (componente `internal/agent`) e "Riscos Conhecidos" (acoplamento ao ctx do turno — risco máximo). Tabela de testes em "Abordagem de Testes → Testes de Integração → `internal/agent`".

## Critérios de Sucesso

- `go test ./internal/agent/ -race` verde
- Subagente que chama bash background registra o processo no **mesmo** manager do pai
- Processo `sleep 30` lançado em background permanece **vivo** após `TurnDone` e após `TurnAborted` (esc)
- `grep -r "procmon" internal/agent/` não retorna imports (direção de dependência preservada)
- `RunSyncPersona` e o registry read-only intocados

## Testes da Tarefa

- [ ] Testes de unidade: N/A (a mudança é de wiring; o comportamento é de integração)
- [ ] Testes de integração (`internal/agent/agent_test.go`, padrão httptest):
  - [ ] `TestSubagentSharesProcMon` — agente com `SetProcMon` → `RunSync` que chama bash background → processo registrado no mesmo manager do pai
  - [ ] `TestBackgroundSurvivesTurnEnd` — turno que lança `sleep 30` em background → turno termina → processo vivo após `TurnDone` (invariante central)
  - [ ] `TestBackgroundSurvivesEsc` — idem + `esc` durante o turno → processo vivo após `TurnAborted` (ctx do turno não mata background)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` (modificado — propagação em `newSubagent`)
- `internal/agent/prompt.go` (modificado — linha do bash no `baseSystemPrompt`)
- `internal/agent/agent_test.go` (modificado — 3 testes novos)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
