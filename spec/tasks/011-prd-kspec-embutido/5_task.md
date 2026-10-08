# Tarefa 5.0: Comandos /kspec-* dinâmicos, tab-completion, hint bar e /kspec-version nativo

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-006 — Slash commands `/kspec-*`
- REQ-009 — Diagnóstico (parcial: `/kspec-version` em sessão)

## Dependências

- 2.0, 3.0, 4.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Tornar os comandos `/kspec-*` dinâmicos sobre as skills resolvidas (projeto-first): despacho no parser existente com turno de kickoff, tab-completion de comandos (novo — hoje só existe completion de menções `@`), `/help` dinâmico, hint bar indicando a skill ativa e `/kspec-version` como display nativo sem turno LLM.

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md` (TUI com receivers por valor), `tests.md`.
</skills>

<requirements>
- `/kspec-<nome> [texto]` ativa a skill correspondente (`ActivateSkill`) e inicia um turno com o texto (ou instrução de início, se vazio); o comando aparece como bloco do usuário na TUI
- Outra `/kspec-*` substitui a skill ativa; comando com skill desconhecida → erro claro; rejeição quando busy
- Tab-completion e `/help` listam dinamicamente as skills resolvidas (projeto-first)
- Hint bar mostra a skill ativa
- `/kspec-version` exibe versão e fonte (`project`/`embedded`) nativamente, sem chamada LLM
</requirements>

## Subtarefas

- [ ] 5.1 Estender `handleCommand` com despacho dinâmico para o prefixo `/kspec-`: resolução no Store, ativação via `ActivateSkill`, turno de kickoff, tratamento de skill desconhecida e busy
- [ ] 5.2 Implementar tab-completion de comandos: popup quando o input começa com `/` (sem espaço), listando comandos nativos + skills resolvidas, reutilizando o padrão do popup de menções (`internal/tui/tui.go:758`)
- [ ] 5.3 Tornar o `/help` dinâmico: seção com os comandos `/kspec-*` resolvidos
- [ ] 5.4 Exibir a skill ativa no hint bar e implementar o display nativo do `/kspec-version` (versão + fonte via `Version()`/`Source()`)
- [ ] 5.5 Passar o Store à TUI no wiring de `main.go`

## Detalhes de Implementação

Seguir a techspec, seção "Visão Geral dos Componentes" (item TUI) e "Decisões Principais" (itens 4 e 8 — display nativo e kickoff na ativação). O parser de comandos atual é um switch estático em `internal/tui/tui.go:876`; o despacho dinâmico entra como caso de prefixo antes do default.

## Critérios de Sucesso

- Os 10 comandos `/kspec-*` funcionam: ativam a skill e disparam o fluxo
- Tab-completion e `/help` refletem as skills resolvidas (projeto ou embutidas)
- Hint bar indica a skill ativa em todos os turnos
- `/kspec-version` mostra versão e fonte sem consumo de tokens

## Testes da Tarefa

- [ ] Testes de unidade
  - Despacho: ativação + kickoff com e sem argumento, skill desconhecida (erro), busy (rejeição), substituição de skill ativa
  - Completion: popup com comandos nativos + skills resolvidas; filtragem por prefixo
  - `/help` dinâmico e hint bar com skill ativa
  - `/kspec-version` nos dois modos de fonte
- [ ] Testes de integração
  - `/kspec-prd` → `ActivateSkill` → system prompt montado contém a skill (fake LLM)
- [ ] Testes E2E (se aplicável)
  - N/A — checklist manual na task 7.0

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go`
- `internal/tui/tui_test.go`
- `main.go` (wiring do Store à TUI)
- `internal/tui/tui.go:758` (referência — popup de completion)
- `internal/tui/tui.go:876` (referência — parser de comandos)
