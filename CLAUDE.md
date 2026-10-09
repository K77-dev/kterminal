# CLAUDE.md

Guia para agentes de IA ao trabalhar com o código deste repositório — plataforma Claude Code.

Este projeto é o **kterminal** — um coding agent de terminal (TUI agêntica) em Go, no estilo Claude Code/opencode. O diferencial: o roteador externo **Jev** (Typesafe) escolhe o LLM a cada chamada, pesando qualidade, custo e tokens/s reais medidos. O desenvolvimento do próprio kterminal segue o fluxo SDD do **kspec** (dogfooding).

> Para OpenAI Codex CLI, consulte `AGENTS.md`. Para Cursor, consulte `CURSOR.md`.

### Idioma

- **Código-fonte**: inglês (identificadores, mensagens de erro, textos da TUI)
- **Specs e documentação de projeto** (briefs `.docs/`, PRD, tech spec, tasks, reviews): português (Brasil)

### Arquitetura

- `main.go` — entry point: flags (`--confirm`, `--continue`, `--session`, `--doctor`), wiring de config/catalog/router/agent/TUI
- `internal/agent/` — loop agêntico; eventos via canal (`EventRoute`, `EventDelta`, `EventToolStart`, `EventToolResult`, `EventConfirm`, `EventTurnDone`, `EventTurnAborted`, `EventError`); subagentes via tool `task` (limite 10 steps / 5 min); compactação de contexto no limiar de 70% (sumário via `deepseek-v4.1-flash`)
- `internal/router/` — interface `Router`; `JevRouter` (API Typesafe systemone, modelo `jev-latest`) com fallback heurístico por keywords
- `internal/llm/` — cliente HTTP próprio, OpenAI-compatible, streaming (`POST /v1/chat/completions`, `GET /v1/models`)
- `internal/catalog/` — catálogo de modelos embutido via `go:embed models.yaml` (default `glm-5.2`)
- `internal/tools/` — registry + tools `read`, `glob`, `grep`, `write`, `edit`, `bash`, `task`; diff colorido em write/edit; hook `go vet` pós-edição de `.go` (`diagnostics.go`)
- `internal/tui/` — TUI Bubble Tea; slash commands (`/quit`, `/exit`, `/help`, `/clear`, `/config`, `/model`, `/image`, `/unimage`); @menções de arquivo; anexo de imagens (5MB, base64); estilo Glamour embutido (`markdown.json`)
- `internal/session/` — transcripts JSONL + snapshots para resume (`~/.local/share/kterminal/sessions/`)
- `internal/config/` — config TOML (`~/.config/kterminal/config.toml`, XDG; env vars `KTERMINAL_LLM_BASE_URL`, `KTERMINAL_LLM_API_KEY`, `TYPESAFE_API_KEY`)
- `internal/telemetry/` — TPS real medido por modelo (`~/.local/share/kterminal/telemetry.json`)

### Comandos do projeto

```bash
go build ./...                 # Build (produz o binário kterminal na raiz)
go run .                       # Rodar em desenvolvimento
go vet ./...                   # Análise estática
gofmt -l .                     # Formatação (não deve listar nenhum arquivo)
go test ./...                  # Testes
```

Verificação completa: `go build ./... && go vet ./... && gofmt -l . && go test ./...`

### Stack

| Camada | Tecnologia | Descrição |
| --- | --- | --- |
| **Linguagem** | Go 1.27 | Módulo `kterminal`, binário único auto-contido |
| **TUI** | Charmbracelet | Bubble Tea, Lipgloss, Glamour, bubbles |
| **LLM** | Cliente HTTP próprio | OpenAI-compatible, streaming |
| **Roteamento** | Jev (Typesafe) | `api.typesafe.ai/v1/systemone` + fallback heurístico |
| **Config** | TOML (BurntSushi) | XDG: `~/.config/kterminal/` |

### Fluxo SDD (dogfooding)

O kterminal é desenvolvido pelo fluxo kspec — toda feature nova segue:

1. Brief da feature em `.docs/NN-<slug>.md` (formato: Contexto / Objetivo / Especificação / Critérios de aceitação / Verificação / Restrições)
2. Skill `kspec-prd` → `spec/tasks/NNN-prd-<slug>/prd.md`
3. Skill `kspec-techspec` → `techspec.md`
4. Skill `kspec-tasks` → `tasks.md` + `N_task.md`
5. Skill `kspec-implement` → implementação sequencial com review por task

As features 001–010 foram implementadas assim: briefs em `.docs/01-*.md` a `10-*.md`, artefatos em `spec/tasks/001-*` a `010-*`.

### Skills kspec

`.agents/` é o source of truth (skills, agents, rules, templates). `.claude/`, `.codex/` e `.cursor/` são camadas de discovery (symlinks e artefatos derivados). Skills de terceiros (mattpocock/skills) são rastreadas em `skills-lock.json`.

No Claude Code, invoque com `/kspec-<nome>`:

| Skill | Função |
| --- | --- |
| `kspec-ideia` | Brainstorm/discovery para decompor ideia em módulos |
| `kspec-prd` | Cria PRD a partir de solicitação de funcionalidade |
| `kspec-techspec` | Traduz PRD em especificação técnica |
| `kspec-tasks` | Quebra Tech Spec em tarefas incrementais |
| `kspec-implement` | Executa todas as tasks pendentes |
| `kspec-qa` | Quality Assurance (E2E, acessibilidade) |
| `kspec-pr-review` | Alinhamento semântico spec × implementação e corpo do PR |
| `kspec-bugfix` | Corrige bugs documentados pelo QA |
| `kspec-bootstrap` | Gera configuração kspec para projeto existente |
| `kspec-version` | Exibe versão atual e lista skills/agents |

### Rules

`.agents/rules/` contém rules do ecossistema kspec (orientadas a TypeScript/Java — `code-standards.md`, `architecture-ddd.md`, `database.md`, `logging.md`, etc.). Para código Go do kterminal, prevalecem os padrões do próprio código-base (seção Restrições dos briefs), não as rules TS.

### Restrições de código Go

- **Sem comentários no código**
- Eventos do agente via canal; TUI com receivers por valor; `strings.Builder` sempre por ponteiro
- Tools registradas no construtor do `Registry` (`internal/tools/tools.go`)
- Assets embutidos via `go:embed` (precedentes: `models.yaml`, `markdown.json`)

### Git

- **Fluxo padrão de entrega**: commitar na branch da feature → fast-forward merge em `develop` → `main` → push das três branches → voltar para `develop` (branch padrão para iniciar novos trabalhos e o fluxo SDD)
- **Não execute** `git restore`, `git reset`, `git clean` ou comandos destrutivos **sem permissão explícita do usuário**
- Binário `kterminal` e `*.test` não são versionados (`.gitignore`)
- **Graphify ativo**: knowledge graph em `graphify-out/graph.json` (não versionado, build code-only). Git hooks (post-commit/post-checkout) o mantêm fresco via `graphify update` (sem LLM). Skills kspec o consultam seguindo `.agents/rules/graphify.md`; edges `INFERRED` são hipótese, `EXTRACTED` são fato

### Limitações conhecidas no Claude Code

1. **Modo interativo obrigatório**: skills como `kspec-prd`, `kspec-techspec`, `kspec-tasks`, `kspec-implement`, `kspec-bugfix` e `kspec-bootstrap` dependem de `AskUserQuestion` — não funcionam em fluxos não-interativos.

2. **Agents**: `kspec-task-runner` e `kspec-qa-runner` exigem permissão de escrita no workspace.

3. **Symlinks em Windows**: em sistemas Windows, `.claude/skills/` pode usar cópias em vez de symlinks.

### Anti-padrões

1. **Pular ativação de skill** — sempre invocar `/kspec-<nome>` quando a task pedir
2. **Editar `.claude/`, `.codex/`, `.cursor/` diretamente** — o source of truth é `.agents/`
3. **Executar comandos git destrutivos sem permissão**
4. **Adicionar comentários ao código Go**
