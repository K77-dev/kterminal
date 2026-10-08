# Tarefa 3.0: System prompt do agente, skill ativa e persistência em sessão

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — System prompt do agente principal
- REQ-004 — Skill ativa persistente
- REQ-008 — Runners via `task` e artefatos no projeto

## Dependências

- 2.0

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

O agente principal ganha assembly de system prompt (prompt base + rules + skill ativa) como **estado derivado**, prefixado no momento da chamada ao LLM e fora de `a.messages`. A skill ativa é ativada por nome, persiste no snapshot de sessão por nome e é re-resolvida projeto-first no resume. O preâmbulo kterminal mapeia os conceitos das skills (AskUserQuestion, runners, artefatos) para os mecanismos do kterminal sem editar o conteúdo vendored.

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md` (eventos via canal, `strings.Builder` por ponteiro), `tests.md`.
</skills>

<requirements>
- System prompt = `[base] + [rules] + [skill]`, montado por `withSystemPrompt()` na hora da chamada — nunca armazenado em `a.messages`
- Rules do projeto entram sempre que existirem; as 13 embutidas apenas como fallback com skill ativa em projeto não-bootstrapped
- Sem rules de projeto e sem skill ativa, o system prompt é apenas o prompt base
- Se `messages[0]` for o resumo de compactação (role system), o assembly o funde no system message único
- `promptChars`/`estimateTokens` somam o system prompt
- Snapshot persiste a skill por NOME; resume re-resolve projeto-first; nome não-resolvível → skill limpa com warning
- `/clear` (Reset) remove a skill ativa
- Preâmbulo kterminal: `AskUserQuestion`/`request_user_input` → tool `ask_user`; agents `kspec-*-runner` → tool `task`; artefatos → `spec/tasks/` do projeto corrente
</requirements>

## Subtarefas

- [ ] 3.1 Escrever o prompt base do kterminal (const em inglês no pacote agent: identidade, tools disponíveis, convenções) e o preâmbulo kterminal (mapeamentos acima)
- [ ] 3.2 Adicionar campos `kspecStore`, `activeSkill`, `systemPrompt` ao `Agent` e métodos `AttachKspec`, `ActivateSkill(name) error`, `ActiveSkill() string`, `ClearSkill()`; assembly do system prompt conforme regras de rules/skill
- [ ] 3.3 Implementar `withSystemPrompt()`: prefixa o system message na chamada ao `ChatStream`, fundindo `messages[0]` system (resumo de compactação) no system único; ajustar `promptChars`/`estimateTokens`
- [ ] 3.4 Ajustar `Reset()` para limpar a skill ativa; registrar evento de sessão `skill_activated` (name, source) na ativação
- [ ] 3.5 Estender sessão: campo `Skill` no `session.Event`, tipo `session.Snapshot{Messages, Skill}`, `Load` retornando `Snapshot`; `RestoreSkill(name)` tolerante no resume; wiring em `main.go`

## Detalhes de Implementação

Seguir a techspec, seções "Visão Geral dos Componentes" (item agent), "Modelos de Dados" (system prompt e snapshot) e "Verificações Técnicas → Arquitetura" (razões do estado derivado e da fusão do resumo). A ativação por slash command acontece na task 5.0 — aqui expõe-se apenas a API do Agent. REQ-008 não tem mecanismo novo: é o preâmbulo (3.1) + skills vendored sem reescrita.

## Critérios de Sucesso

- Com rules de projeto presentes, elas estão no system prompt em todos os turnos
- Sem rules de projeto e sem skill ativa, o prompt é apenas o base
- Skill ativa persiste entre turnos sem reinjeção manual; substituída por outra ativação; removida por `Reset`
- Após resume, a skill ativa é restaurada (re-resolvida); snapshot de sessão contém o nome
- Compactação não destrói o system prompt nem produz dois system messages na chamada

## Testes da Tarefa

- [ ] Testes de unidade
  - Assembly: base puro / base + rules de projeto / base + skill + rules embutidas fallback
  - Ativação, substituição por outra skill e clear via `Reset`
  - `withSystemPrompt` funde resumo de compactação (messages[0] system) no system único
  - `promptChars` inclui o system prompt
- [ ] Testes de integração
  - Snapshot roundtrip: sessão com skill ativa → `Load` → `RestoreSkill` → prompt re-resolvido
  - Nome não-resolvível no resume → skill limpa + warning, sem erro fatal
- [ ] Testes E2E (se aplicável)
  - N/A — superfície de comando vem na task 5.0

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go`
- `internal/agent/prompt.go` (novo)
- `internal/agent/agent_test.go`
- `internal/session/session.go`
- `internal/session/session_test.go`
- `main.go` (wiring e resume)
- `internal/agent/agent.go:256` (referência — precedente `AttachTaskTool`)
