# Tarefa 3.0: Loop de auto-correção no Agent e injeção no `main.go`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Correção autônoma no mesmo turno

## Dependências

- 2.0 (hook `OnGoEdit` chamado pelo registry)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

REQ-004 é consequência, não código novo: o diagnóstico anexado ao `Output` circula de volta ao LLM como parte da mensagem `role: "tool"` pelo fluxo existente — o modelo reage ao texto e corrige o erro no passo seguinte. Esta task valida o loop com o teste mandatório de auto-correção (mock gateway com hook fake) e faz a injeção da composição real: `reg.OnGoEdit = tools.GoVetHook` no `main.go` (o pacote `tools` permanece testável sem Go instalado). O agent, a TUI e a session **não mudam** — o texto anexado circula como qualquer tool result.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (auto-correção visível no mesmo turno), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; `main.go` como ponto de composição; zero mudanças em agent/TUI/session.
</skills>

<requirements>
- `main.go`: `reg.OnGoEdit = tools.GoVetHook` (ou via `SetOnGoEdit`) na inicialização.
- O teste mandatório prova o loop completo: 1ª resposta chama `edit` que quebra (hook fake retorna `Diagnostics:\n...undefined: Foo`); a 2ª chamada ao gateway recebe a mensagem `role: "tool"` contendo o diagnóstico; 2ª resposta fecha o turno — sem input do usuário.
- Nenhum código novo em `internal/agent`, `internal/tui` ou `internal/session` — o requisito é validado, não implementado.
</requirements>

## Subtarefas

- [ ] 3.1 Escrever `TestAgentSelfCorrectsWithinTurn` com mock gateway e hook fake (ver Testes da Tarefa)
- [ ] 3.2 Injetar `reg.OnGoEdit = tools.GoVetHook` no `main.go`
- [ ] 3.3 Confirmar por inspeção que agent/TUI/session não mudaram (fluxo existente carrega o diagnóstico)

## Detalhes de Implementação

- Cenário do teste de auto-correção: techspec, seção **Abordagem de Testes → Testes Unidade** (item 12).
- Fluxo de dados (Output → EventToolResult → `role: "tool"` → modelo corrige): techspec, seção **Arquitetura do Sistema → Fluxo de dados**.
- Injeção no `main.go`: techspec, seção **Arquitetura do Sistema** (componente `main.go`) e **Sequenciamento de Desenvolvimento** (passo 4).
- Interação com a techspec 007: se `ExecuteStream` existir, o hook roda no fluxo consolidado (após o fim da execução), independente de `onLine` — techspec, **Dependências Técnicas**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste mandatório prova: em um turno completo, a edição que quebra o código é seguida de correção pelo próprio agente — a 2ª chamada ao gateway contém a mensagem `role: "tool"` com o diagnóstico; o turno fecha sem input do usuário.
- O `main.go` injeta o hook real; os testes do pacote `tools` continuam passando sem Go instalado.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`):
  - `TestAgentSelfCorrectsWithinTurn` (REQ-004, mandatório) — mock gateway: 1ª resposta chama `edit` que quebra (hook fake → `Diagnostics:\n...undefined: Foo`); mock captura a 2ª chamada com a mensagem `role: "tool"` contendo o diagnóstico; 2ª resposta fecha o turno (techspec, item 12).
- [ ] Testes de integração — cobertos por 12 (agent + registry + hook fake, fluxo ponta a ponta do turno, conforme techspec **Abordagem de Testes → Testes de Integração**).
- [ ] Testes E2E — deferidos para `kspec-qa` (edição real que quebra → diagnóstico visível → agente corrige sozinho; edição válida → `Diagnostics: clean`; fora de módulo → sem diagnóstico).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent_test.go` — teste de auto-correção no mesmo turno (REQ-004)
- `main.go` — injeção do hook na inicialização
- `internal/tools/diagnostics.go` — `GoVetHook` injetado (task 1.0)
- `internal/tools/tools.go` — ponto de chamada do hook (task 2.0)
