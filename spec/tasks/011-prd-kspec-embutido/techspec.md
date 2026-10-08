# Tech Spec — kspec embutido no kterminal

## Requisitos Atendidos

- REQ-001 — Conteúdo kspec embutido (vendoring)
- REQ-002 — Loader com precedência projeto-first
- REQ-003 — System prompt do agente principal
- REQ-004 — Skill ativa persistente
- REQ-005 — Tool `ask_user`
- REQ-006 — Slash commands `/kspec-*`
- REQ-007 — `/kspec-bootstrap` interoperável
- REQ-008 — Runners via `task` e artefatos no projeto
- REQ-009 — Diagnóstico (`--doctor`, `/kspec-version`)

## Resumo Executivo

Novo pacote `internal/kspec` expõe a cópia vendored do kspec upstream (`internal/kspec/embed/`) via `go:embed`, com parse de frontmatter, resolução projeto-first por componente (`.agents/` do cwd vence) e API `List/Resolve/Rules/Version`. O `Agent` ganha assembly de system prompt (prompt base + rules + skill ativa) como **estado derivado**, prefixado no momento da chamada ao LLM e fora de `a.messages` — compactação, snapshot e resume permanecem intactos; a skill ativa persiste no snapshot por **nome** e é re-resolvida projeto-first no resume.

A tool `ask_user` segue o precedente do fluxo de confirmação write/edit (evento + canal de resposta bloqueante), com wizard sequencial na TUI. `/kspec-bootstrap` materializa a árvore embutida via tool Go nativa `kspec_bootstrap` (escrita determinística, sem custo de tokens); `/kspec-version` é display nativo sem turno LLM. Os comandos `/kspec-*` passam a despacho dinâmico sobre as skills resolvidas, com tab-completion novo reutilizando o padrão do popup de menções.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/kspec` (novo)** — `Store`: `go:embed embed`, parse de frontmatter (name, version, description, argument-hint), resolução projeto-first, inlining de templates referenciados, materialização de bootstrap. Instanciado em `main.go` e compartilhado por Agent e TUI.
- **`internal/kspec/embed/` (novo)** — árvore vendored: `skills/kspec-*/SKILL.md`, `templates/*.md`, `rules/*.md`, `VERSION`. Única via de entrada: `scripts/sync-kspec.sh`.
- **`internal/agent` (modificado)** — campos `kspecStore`, `activeSkill`, `systemPrompt`; métodos `AttachKspec`, `ActivateSkill`, `ActiveSkill`, `ClearSkill`, `RestoreSkill`, `AttachAskUserTool`; assembly `withSystemPrompt()`; novo `EventAskUser`.
- **`internal/tools` (modificado)** — `ask_user` anexada pelo Agent (precedente `AttachTaskTool`, agent.go:256); `kspec_bootstrap` registrada no construtor do `Registry` (importa `internal/kspec`).
- **`internal/tui` (modificado)** — despacho dinâmico `/kspec-*`, completion de comandos, estado `stateAsk` (wizard), hint bar com skill ativa, `/help` e `/kspec-version` dinâmicos.
- **`internal/session` (modificado)** — campo `Skill` no evento de snapshot; `Load` retorna `Snapshot{Messages, Skill}`.
- **`main.go` (modificado)** — wiring do Store, seção kspec no `--doctor`, restore da skill no resume.

Fluxo: usuário digita `/kspec-prd <texto>` → TUI resolve a skill no Store → `agent.ActivateSkill` monta o system prompt → TUI dispara turno de kickoff → agente clarifica via `ask_user` (evento → wizard na TUI → respostas pelo canal) → artefatos gravados em `spec/tasks/NNN-prd-<slug>/` pelas tools de fs existentes.

## Design de Implementação

### Interfaces Principais

```go
type Skill struct {
    Name, Version, Description, ArgHint string
}

func Load() *Store
func (s *Store) List() []Skill
func (s *Store) Resolve(name string) (string, error)
func (s *Store) Rules() []Rule
func (s *Store) Version() string
func (s *Store) Source() string
func (s *Store) Bootstrap(dir string, force bool) error
```

`Resolve` devolve o SKILL.md com os templates referenciados (`@.agents/templates/*.md`) inlined, cada um resolvido projeto-first. `Source()` retorna `"project"` (se existe `.agents/skills/kspec-*/SKILL.md` no cwd) ou `"embedded"`; `Version()` devolve o `VERSION` embutido ou o frontmatter da skill `kspec-version` do projeto, conforme a fonte.

Agent:

```go
func (a *Agent) AttachKspec(s *kspec.Store)
func (a *Agent) ActivateSkill(name string) error
func (a *Agent) ActiveSkill() string
func (a *Agent) RestoreSkill(name string)
func (a *Agent) AttachAskUserTool()
```

Schema da tool `ask_user`:

```json
{"questions": [{"question": "...", "header": "...",
  "multiple": false,
  "options": [{"label": "...", "description": "..."}]}]}
```

### Modelos de Dados

- `agent.AskQuestion{Question, Header string; Options []AskOption; Multiple bool}`; `AskOption{Label, Description string}`.
- `Event` ganha `Questions []AskQuestion` e `AnswerCh chan []string` — uma resposta por pergunta, na ordem; multi-seleção junta labels com `", "`; cancelamento (esc) fecha com `nil` e a tool devolve `"user declined to answer"`.
- `session.Event` ganha `Skill string \`json:"skill,omitempty"\``; novo tipo `session.Snapshot{Messages []llm.Message; Skill string}`; `Load` passa a retorná-lo.
- **System prompt** (derivado, nunca persistido em mensagens): `[base] + [rules] + [skill]`. Rules do projeto (`.agents/rules/*.md`) entram sempre que existirem; as 13 embutidas entram como fallback apenas com skill ativa em projeto não-bootstrapped. Skill = preâmbulo kterminal + SKILL.md + templates inlined.
- **Preâmbulo kterminal** (const fixa que envolve o conteúdo vendored sem editá-lo): mapeia `AskUserQuestion`/`request_user_input` → tool `ask_user`; agents `kspec-*-runner` → tool `task` (description + guidance); artefatos → `spec/tasks/` do projeto corrente. Garante que as skills interoperáveis funcionem no kterminal sem reescrita.

### Endpoints de API

N/A — funcionalidade inteiramente local ao binário.

## Pontos de Integração

- **Upstream kspec** (`K77-dev/kspec`): integração apenas em build-time via `scripts/sync-kspec.sh` — clone do repo, cópia de `.agents/skills/kspec-*/`, `.agents/templates/`, `.agents/rules/` e `VERSION` para `internal/kspec/embed/`, com carimbo de versão; exclui skills de terceiros e `skills-lock.json`. Sem dependência de runtime, sem auto-sync.
- **Interoperabilidade multi-harness**: o bootstrap escreve a estrutura kspec padrão; nenhum formato muda.

## Verificações Técnicas

### Segurança

- `kspec_bootstrap` é mutante (sujeita ao confirm de `--confirm`) e se recusa a sobrescrever `.agents/` existente sem `force: true` — protege customizações locais.
- `ask_user` não transporta dados sensíveis; respostas voltam como tool result e entram no transcript como qualquer outra.
- Conteúdo vendored é imutável em runtime (`embed.FS` read-only); a injeção de prompt não interpreta o conteúdo.

### Arquitetura

- System prompt prefixado em `withSystemPrompt()` na hora da chamada; `a.messages` permanece limpo. Se `messages[0]` for o resumo de compactação (role system), o assembly o funde no system message único — evita dois system messages em gateways OpenAI-compatible.
- `promptChars`/`estimateTokens` passam a somar o system prompt (o custo das rules fica visível ao threshold de compactação).
- Skill ativa persistida por nome e re-resolvida no resume — correto se o projeto foi bootstrapped entre sessões; nome não-resolvível → skill limpa com warning.
- Direção de dependências: `tools` → `kspec`, `agent` → `kspec`, `tui` → `kspec` (apenas listagem/versão). `kspec` não importa agent/tools/tui.

### Infraestrutura

- Binário único cresce ~190 KB (conteúdo kspec); sem novos requisitos de deploy ou serviços externos.
- Rollback: remover o wiring em `main.go` desativa tudo; a árvore vendored é inerte sem o loader.

## Abordagem de Testes

### Testes de Unidade

- `internal/kspec`: parse de frontmatter; `List`/`Resolve`/`Rules`/`Version` sobre embed; precedência projeto-first com temp dir simulando `.agents/`; inlining de templates; `Bootstrap` materializa a árvore + guard de overwrite.
- `internal/agent`: assembly do system prompt (base puro / base + rules de projeto / base + skill + rules fallback); ativação, substituição e clear; `withSystemPrompt` fundindo resumo de compactação; snapshot roundtrip com `Skill`.
- `internal/tools`: contrato `ask_user` (validação de args, bloqueio no canal, formatação do result); `kspec_bootstrap` (escrita, `force`, erro em dir existente).
- `internal/tui`: despacho `/kspec-*` (ativação + kickoff, skill desconhecida, rejeição com busy), `/help` e completion dinâmicos, wizard (navegação, multi-seleção, texto livre, cancelamento), hint bar.

Mocks apenas onde já existem (fake LLM/roteador); o `Store` é testável com temp dirs, sem mock.

### Testes de Integração

- Ativação de skill → turno com fake LLM → system prompt montado presente na primeira mensagem da chamada; `ask_user` → evento → resposta via canal → tool result no transcript.
- Resume: sessão com skill ativa → `Load` → `RestoreSkill` → prompt re-resolvido.

### Testes de E2E

TUI sem frontend web — TestSprite não se aplica. E2E = checklist manual no QA: `/kspec-prd` em projeto vazio (fluxo completo com `ask_user`, artefato em `spec/tasks/`), projeto com `.agents/` (precedência), bootstrap validado no Claude Code, `--continue` com skill ativa, `--doctor`.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. `scripts/sync-kspec.sh` + árvore `internal/kspec/embed/` + loader com testes — fundação inerte, zero mudança de comportamento.
2. Assembly de system prompt + skill ativa + persistência em sessão (REQ-003/004) — depende do loader.
3. Tool `ask_user` + evento + wizard na TUI (REQ-005) — paralelizável com o passo 2.
4. Comandos `/kspec-*` dinâmicos + tab-completion + `/help` + hint bar + `/kspec-version` nativo (REQ-006, REQ-09 parcial) — depende de 2 e usa 3.
5. Tool `kspec_bootstrap` (REQ-007) — depende do loader; usa `ask_user` para plataformas.
6. `--doctor` + wiring final + verificação completa (REQ-009).

### Dependências Técnicas

- Feature 010 (tool `task`) — já implementada. REQ-008 não exige mecanismo novo: o preâmbulo do passo 2 mapeia os runners do kspec para a tool `task` existente.

## Monitoramento e Observabilidade

### Error Tracking

Sem Sentry (app local). Erros de ativação/resolução de skill e falhas de `ask_user`/`kspec_bootstrap` fluem pelo `EventError` existente → transcript JSONL (`type: "error"`).

### Logging Estruturado

Transcript JSONL existente como fonte de verdade. Eventos novos: snapshot ganha `skill`; ativação registra evento de sessão `skill_activated` (name, source). Nenhum dado sensível (API keys) transita por esses eventos.

### Health Checks

`--doctor` ganha seção `kspec: v{X.Y.Z} ({source})` — versão e fonte ativa no cwd. É o readiness check do fluxo SDD.

### Métricas de Negócio

O custo do system prompt com skill ativa fica observável pelos eventos de rota/custo existentes (prompt tokens por turno). KPI: conclusão de fluxos SDD sem `/clear` (análise offline de transcripts).

### Alertas

N/A — aplicação local interativa; erros são superfície direta na TUI.

## Considerações Técnicas

### Decisões Principais

1. **System prompt como estado derivado** (prefixado na chamada, fora de `messages`). Alternativas rejeitadas: system message em `messages[0]` (destruído pela compactação, exigiria caso especial) ou mudança no `llm.Client` (maior raio de mudança).
2. **Persistência por nome + re-resolução no resume**: o conteúdo pode divergir entre sessões (bootstrap no meio do caminho); o nome é o contrato estável.
3. **`kspec_bootstrap` como tool Go nativa** (decisão do usuário): escrita determinística de `.agents/`, `spec/tasks/` e `VERSION` sem consumir contexto; o agente permanece responsável pela análise do projeto, escolha de plataformas via `ask_user` e geração dos `*.bootstrap.md` a partir dos templates.
4. **`/kspec-version` como display nativo** (decisão do usuário): informação estática sem turno LLM; a skill vendored segue existindo para outros harnesses.
5. **Todas as 13 rules embutidas como fallback** (decisão do usuário): fidelidade ao PRD e paridade entre harnesses.
6. **`ask_user` via método do Agent, só no principal** (decisão do usuário): precedente de `AttachTaskTool`; subagentes permanecem autônomos (não bloqueiam esperando usuário).
7. **Wizard sequencial — uma pergunta por tela** (decisão do usuário): reutiliza o padrão visual do popup de menções.
8. **Kickoff na ativação**: `/kspec-<nome> [texto]` ativa a skill e inicia um turno com o texto (ou instrução de início, se vazio) — um único comando roda o fluxo; o comando aparece como bloco do usuário na TUI.
9. **Preâmbulo kterminal em volta do conteúdo vendored**: mapeia `AskUserQuestion` → `ask_user` e runners → `task` sem editar as skills, que permanecem interoperáveis.

### Riscos Conhecidos

- **Tokens do system prompt**: 13 rules (~82 KB) + skill (bootstrap ~30 KB) ≈ 25–30k tokens por turno em projeto não-bootstrapped. Mitigação: compactação existente; curadoria de rules é evolução futura, não escopo.
- **`maxSteps` 25 no agente principal**: `kspec-implement` com muitas tasks (task + review por task) pode atingir o limite. Mitigação: runners fazem o trabalho pesado (10 steps internos cada); elevar o limite com skill ativa é ajuste de constante, se necessário.
- **Gateways com system messages duplicados**: endereçado pela fusão do resumo de compactação no system prompt único.
- **Drift vendored ↔ upstream**: só entra via `scripts/sync-kspec.sh`; edições manuais em `embed/` são bloqueadas por convenção documentada no script.

### Conformidade com Skills Padrões

CLAUDE.md não traz tabela "Stack e skills recomendadas". Rules aplicáveis do repo: `code-standards.md` (sem comentários, padrões Go do código-base), `tests.md` (cobertura dos pacotes novos/alterados). `architecture-ddd.md` não se aplica — projeto Go sem bounded contexts `src/modules/`; a seção do template foi removida conforme o próprio template determina.

### Arquivos relevantes e dependentes

- **Novos**: `scripts/sync-kspec.sh`; `internal/kspec/{kspec.go, frontmatter.go, bootstrap.go, kspec_test.go}`; `internal/kspec/embed/**`; `internal/tools/{ask.go, ask_test.go, bootstrap.go, bootstrap_test.go}`; `internal/tui/ask.go`.
- **Modificados**: `main.go`; `internal/agent/{agent.go, agent_test.go}` (+ `prompt.go`); `internal/tools/tools.go`; `internal/tui/{tui.go, tui_test.go}`; `internal/session/{session.go, session_test.go}`.
- **Referência (precedentes)**: `internal/catalog/catalog.go:11` (embed), `internal/agent/agent.go:256` (attach de tool), `internal/tui/tui.go:1131` (fluxo bloqueante), `internal/tui/tui.go:758` (popup de completion), `.docs/11-kspec-embutido.md`.
