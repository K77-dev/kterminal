# 11 — kspec embutido no kterminal

## Contexto

kterminal é um terminal agêntico em Go (Bubble Tea) onde o Jev (Typesafe) escolhe o LLM a cada chamada. O kspec (repo `K77-dev/kspec`, pacote npm `@k77-dev/kspec`) é o kit SDD usado para desenvolver o próprio kterminal via dogfooding — hoje instalado alongside no repo (`.agents/`, `spec/tasks/`), utilizável apenas por harnesses externos (Claude Code, Codex CLI, Cursor). Arquitetura relevante:

- `internal/agent/agent.go` — o agente principal **não tem system prompt**: o loop só faz append da mensagem do usuário (`agent.go:309`); apenas o subagente tem system prompt fixo. Subagentes via tool `task` (`agent.go:256-288`), limite 10 steps / 5 min.
- `internal/tools/tools.go` — `Registry` com tools fixas registradas no construtor (`read`, `glob`, `grep`, `write`, `edit`, `bash`, `task`); sem loading dinâmico.
- `internal/tui/tui.go` — parser de slash commands (`/quit`…`/unimage`, `tui.go:876-946`); fluxo de confirmação de write/edit como precedente de interação bloqueante no loop.
- `internal/catalog/catalog.go` e `internal/tui/styles.go` — precedentes de assets embutidos via `go:embed` (`models.yaml`, `markdown.json`).
- `internal/session/session.go` — transcripts JSONL + snapshots para resume.
- `.agents/` — kspec instalado: 10 skills `kspec-*`, 3 agents (runners), 14 rules, 7 templates; `.claude/`, `.codex/`, `.cursor/` são camadas de discovery.

## Objetivo

Embutir o kspec no binário do kterminal: as 10 skills `kspec-*`, os templates e as rules passam a ser assets `go:embed`, invocáveis nativamente por slash commands `/kspec-*`, sem que o usuário final precise instalar o pacote npm ou usar outro harness. O kterminal torna-se um harness SDD completo: ideia → PRD → techspec → tasks → implementação → review → QA.

## Especificação

1. **Vendoring**: novo diretório `internal/kspec/embed/` com a cópia do conteúdo kspec (10 skills `kspec-*`, `templates/`, `rules/`, `VERSION`). Script `scripts/sync-kspec.sh` clona `K77-dev/kspec`, copia o conteúdo e carimba a versão. Skills de terceiros (mattpocock, `skills-lock.json`) ficam fora.

2. **Loader** (`internal/kspec/`): `go:embed` da árvore; parse de frontmatter das skills (name, version, description); API `List()`, `Load(name)`, `Rules()`, `Version()`.

3. **Precedência projeto-first**: na resolução de cada componente (skill, template, rule), `.agents/` do projeto corrente vence; o conteúdo embutido é fallback. Projetos com kspec instalado usam as cópias (e a versão) do projeto.

4. **System prompt no agente principal**: o `Agent` ganha assembly de system prompt = prompt base do kterminal + rules + skill ativa. Rules do projeto (`.agents/rules/*.md`) entram sempre que existirem (comportamento análogo ao CLAUDE.md no Claude Code); as rules embutidas só entram como fallback enquanto uma skill estiver ativa em projeto não-bootstrapped.

5. **Skill ativa persistente**: ao invocar `/kspec-<nome>`, o SKILL.md + templates referenciados tornam-se o system prompt da sessão e persistem em todos os turnos até a skill completar, `/clear`, ou outra `/kspec-*` substituí-la. A skill ativa faz parte do snapshot de sessão (sobrevive a `--continue`).

6. **Tool `ask_user`**: nova tool estruturada de perguntas, sempre disponível (não só em skills). Entrada: `questions[]` com `question`, `header`, `options[]` (label + description), `multiple`. TUI renderiza prompt bloqueante com navegação por setas, seleção múltipla e campo de texto livre como fallback — mesmo mecanismo de eventos do fluxo de confirmação write/edit.

7. **Slash commands**: `/kspec-ideia`, `/kspec-prd`, `/kspec-techspec`, `/kspec-tasks`, `/kspec-implement`, `/kspec-qa`, `/kspec-pr-review`, `/kspec-bugfix`, `/kspec-bootstrap`, `/kspec-version` — via parser existente; tab-completion e `/help` listam dinamicamente as skills resolvidas (projeto-first).

8. **`/kspec-bootstrap` interoperável**: escreve estrutura kspec completa no projeto — `.agents/` (skills, rules, templates), `spec/tasks/`, e `*.bootstrap.md` das plataformas escolhidas via `ask_user` (matriz de plataformas do kspec). O projeto torna-se um projeto kspec padrão, usável também por Claude Code/Cursor/Codex. Pós-bootstrap, as cópias do projeto passam à frente das embutidas.

9. **Runners via `task`**: as skills `kspec-implement`, `kspec-qa` e `kspec-pr-review` instruem o agente a despachar cada task/review pela tool `task` existente (subagente com contexto isolado). Os agents do kspec não viram arquivos nem mecanismo novo.

10. **Artefatos no projeto**: PRDs/techspecs/tasks/reviews gerados pelas skills vão para `spec/tasks/NNN-prd-<slug>/` na raiz do projeto corrente (convenção kspec). As cópias embutidas das skills funcionam sem reescrita de caminhos.

11. **`--doctor`**: reporta a versão do kspec embutida e qual fonte está ativa no projeto corrente (projeto vs embutida).

## Critérios de aceitação

- `/kspec-prd` num projeto sem kspec: fluxo completo roda no kterminal — perguntas estruturadas via `ask_user`, artefato escrito em `spec/tasks/` do projeto, skill persistente entre turnos.
- Projeto com `.agents/`: as skills/rules/templates do projeto vencem as embutidas; `/kspec-version` reflete a versão do projeto.
- `/kspec-bootstrap` gera estrutura kspec interoperável (abrir o mesmo projeto no Claude Code enxerga as skills).
- `ask_user` utilizável em qualquer conversa, com múltiplas perguntas por chamada, opções navegáveis e texto livre.
- Skill ativa sobrevive a `--continue` (resume de sessão).
- `--doctor` exibe "kspec vX.Y.Z (embedded)" ou "(project)" conforme a fonte.
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passa.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes novos: `internal/kspec/` (precedência projeto × embutido, parse de frontmatter, versionamento), `internal/tools/` (contrato da tool `ask_user`), `internal/agent/` (assembly de system prompt: base + rules + skill ativa), `internal/tui/` (parsing dos comandos `/kspec-*`, listagem dinâmica).

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Padrões existentes: eventos do agente via canal, TUI com receivers por valor, `strings.Builder` sempre por ponteiro, tools registradas no construtor do `Registry`.
- O conteúdo embutido é cópia vendored do kspec upstream — edições de conteúdo acontecem no repo kspec; aqui só entra via `scripts/sync-kspec.sh`.
- Artefatos e documentação de spec em português (Brasil); código-fonte em inglês.
- Não modificar o comportamento das tools existentes nem o protocolo de eventos além do necessário (`ask_user` adiciona eventos novos sem quebrar os existentes).
