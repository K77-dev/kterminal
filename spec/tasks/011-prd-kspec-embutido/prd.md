# PRD — kspec embutido no kterminal

## Visão Geral

O kterminal é um coding agent de terminal, mas o fluxo SDD (ideia → PRD → techspec → tasks → implementação → review → QA) que guia seu desenvolvimento hoje depende do kit kspec instalado alongside no projeto (`.agents/`, `spec/tasks/`) e de um harness externo (Claude Code, Codex CLI, Cursor) para executar as skills. Um usuário final do kterminal sem esse kit não tem acesso ao fluxo SDD; quem tem, precisa alternar entre ferramentas.

Este PRD define o kspec embutido: as 10 skills `kspec-*`, os templates e as rules do kspec passam a ser assets `go:embed` no binário do kterminal, invocáveis nativamente por slash commands `/kspec-*`. O kterminal torna-se um harness SDD completo e auto-contido — sem instalação de npm, sem harness externo — mantendo interoperabilidade total com projetos kspec existentes (precedência projeto-first).

## Objetivos

- **SDD nativo E2E**: o fluxo completo ideia → PRD → techspec → tasks → implementação → review → QA roda 100% no kterminal, num projeto sem kspec instalado.
- **Zero instalação**: nenhum requisito externo (npm, harness) para usar as skills kspec no kterminal.
- **Interoperabilidade**: projetos bootstrapped/gerados pelo kterminal são projetos kspec padrão, usáveis por Claude Code, Codex CLI e Cursor.
- **Precedência projeto-first**: projetos com kspec instalado usam as cópias (e a versão) do projeto; o conteúdo embutido é fallback.
- **Persistência**: a skill ativa sobrevive entre turnos e ao resume de sessão (`--continue`).

## Histórias de Usuário

- Como desenvolvedor usando kterminal num projeto qualquer, quero digitar `/kspec-prd` e rodar o fluxo SDD completo, para que eu não precise instalar nada nem abrir outro harness.
- Como desenvolvedor com kspec já instalado no projeto, quero que as skills/rules/templates do projeto vençam os embutidos, para que customizações locais sejam respeitadas.
- Como desenvolvedor multi-harness, quero que `/kspec-bootstrap` gere uma estrutura kspec padrão, para que o mesmo projeto funcione no Claude Code e no Cursor.
- Como usuário em qualquer conversa, quero responder perguntas estruturadas do agente (opções navegáveis, seleção múltipla, texto livre), para que interações de clarificação sejam rápidas e sem ambiguidade.
- Como usuário de sessão retomada, quero que a skill ativa sobreviva ao `--continue`, para que um fluxo SDD interrompido continue de onde parou.
- Como usuário, quero ver qual skill está ativa (hint bar) e a versão/fonte do kspec (`/kspec-version`, `--doctor`), para que eu saiba sempre o estado do fluxo.

## Funcionalidades Principais

### REQ-001 — Conteúdo kspec embutido (vendoring)

Cópia vendored do kspec upstream no repositório do kterminal, em `internal/kspec/embed/`: as 10 skills `kspec-*`, `templates/`, `rules/` e `VERSION`. Um script `scripts/sync-kspec.sh` clona `K77-dev/kspec`, copia o conteúdo e carimba a versão — única via de entrada de conteúdo (edições acontecem no repo kspec).

#### Critérios de Aceite

- O binário contém skills, templates, rules e versão do kspec via `go:embed`.
- `scripts/sync-kspec.sh` atualiza o conteúdo vendored de forma reproduzível.
- Skills de terceiros (mattpocock, `skills-lock.json`) ficam de fora.

---

### REQ-002 — Loader com precedência projeto-first

Loader em `internal/kspec/` faz `go:embed` da árvore, parseia frontmatter das skills (name, version, description) e expõe `List()`, `Load(name)`, `Rules()`, `Version()`. Na resolução de cada componente (skill, template, rule), `.agents/` do projeto corrente vence; o conteúdo embutido é fallback.

#### Critérios de Aceite

- Em projeto sem `.agents/`, as skills embutidas resolvem; em projeto com `.agents/`, as cópias do projeto (e sua versão) prevalecem.
- Frontmatter das skills é parseado (name, version, description).

---

### REQ-003 — System prompt do agente principal

O agente principal ganha assembly de system prompt: prompt base do kterminal + rules + skill ativa. Rules do projeto (`.agents/rules/*.md`) entram sempre que existirem; as rules embutidas entram apenas como fallback enquanto uma skill estiver ativa em projeto não-bootstrapped.

#### Critérios de Aceite

- Com rules de projeto presentes, elas estão no system prompt em todos os turnos.
- Sem rules de projeto e sem skill ativa, o system prompt é apenas o prompt base.

---

### REQ-004 — Skill ativa persistente

Ao invocar `/kspec-<nome>`, o SKILL.md + templates referenciados tornam-se parte do system prompt da sessão e persistem em todos os turnos até a skill completar, `/clear`, ou outra `/kspec-*` substituí-la. A skill ativa integra o snapshot de sessão e sobrevive a `--continue`.

#### Critérios de Aceite

- A skill ativa persiste entre turnos sem reinjeção manual pelo usuário.
- Após `--continue`, a skill ativa é restaurada.
- `/clear` ou outra `/kspec-*` substitui/remove a skill ativa.

---

### REQ-005 — Tool `ask_user`

Nova tool estruturada de perguntas, sempre disponível ao agente (não só com skill ativa). Entrada: `questions[]` com `question`, `header`, `options[]` (label + description), `multiple`. A TUI renderiza prompt bloqueante com navegação por setas, seleção múltipla e campo de texto livre como fallback — mesmo mecanismo de eventos do fluxo de confirmação write/edit.

#### Critérios de Aceite

- O agente pode chamar `ask_user` em qualquer conversa, com múltiplas perguntas por chamada.
- A TUI oferece navegação por teclado, seleção múltipla e texto livre.
- A resposta estruturada volta ao agente como tool result.

---

### REQ-006 — Slash commands `/kspec-*`

Os 10 comandos — `/kspec-ideia`, `/kspec-prd`, `/kspec-techspec`, `/kspec-tasks`, `/kspec-implement`, `/kspec-qa`, `/kspec-pr-review`, `/kspec-bugfix`, `/kspec-bootstrap`, `/kspec-version` — via parser existente. Tab-completion e `/help` listam dinamicamente as skills resolvidas (projeto-first). O hint bar mostra a skill ativa.

#### Critérios de Aceite

- Os 10 comandos funcionam e ativam a skill correspondente.
- Tab-completion e `/help` refletem as skills resolvidas (projeto ou embutidas).
- O hint bar indica a skill ativa.

---

### REQ-007 — `/kspec-bootstrap` interoperável

Escreve a estrutura kspec completa no projeto corrente: `.agents/` (skills, rules, templates), `spec/tasks/` e `*.bootstrap.md` das plataformas escolhidas via `ask_user` (matriz de plataformas do kspec). O projeto torna-se um projeto kspec padrão, usável também por Claude Code/Cursor/Codex; pós-bootstrap, as cópias do projeto passam à frente das embutidas.

#### Critérios de Aceite

- Projeto bootstrapped pelo kterminal é reconhecido pelo Claude Code (skills visíveis).
- Plataformas escolhidas via `ask_user` definem quais `*.bootstrap.md` são gerados.

---

### REQ-008 — Runners via `task` e artefatos no projeto

As skills `kspec-implement`, `kspec-qa` e `kspec-pr-review` instruem o agente a despachar cada task/review pela tool `task` existente (subagente com contexto isolado) — os agents do kspec não viram arquivos nem mecanismo novo. Artefatos gerados (PRDs, techspecs, tasks, reviews) vão para `spec/tasks/NNN-prd-<slug>/` na raiz do projeto corrente, sem reescrita de caminhos nas skills embutidas.

#### Critérios de Aceite

- Implementação/QA/review delegam por `task` com contexto isolado.
- Artefatos são escritos em `spec/tasks/` do projeto corrente pela convenção kspec.

---

### REQ-009 — Diagnóstico (`--doctor`, `/kspec-version`)

`--doctor` reporta a versão do kspec embutida e qual fonte está ativa no projeto corrente (projeto vs embutida). `/kspec-version` mostra o mesmo em sessão.

#### Critérios de Aceite

- `--doctor` exibe "kspec vX.Y.Z (embedded)" ou "(project)" conforme a fonte ativa.

## Experiência do Usuário

- **Fluxo SDD sem fricção**: `/kspec-prd` num projeto qualquer dispara clarificação via `ask_user` (setas + Enter, múltipla escolha, texto livre), seguida da redação e gravação do artefato — tudo nativo.
- **Estado visível**: hint bar mostra a skill ativa; `/kspec-version` e `--doctor` revelam versão e fonte.
- **Acessibilidade**: todas as interações `ask_user` são 100% navegáveis por teclado; texto livre é sempre um fallback válido — o fluxo nunca bloqueia por indisponibilidade de opção.
- **Continuidade**: interrupções (fechar terminal, retomar depois) não perdem a skill ativa nem o progresso do fluxo.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- Padrões existentes não negociáveis: eventos do agente via canal, TUI com receivers por valor, `strings.Builder` por ponteiro, tools registradas no construtor do `Registry`, assets via `go:embed`.
- O conteúdo embutido é cópia vendored — só entra via `scripts/sync-kspec.sh`; edições de conteúdo acontecem no repo kspec.
- Não modificar o comportamento das tools existentes nem quebrar o protocolo de eventos (`ask_user` adiciona eventos novos sem quebrar os existentes).
- Dependência da feature 010 (tool `task`) para os runners.
- Artefatos e documentação de spec em português (Brasil); código-fonte em inglês.

## Fora de Escopo

- Skills de terceiros (mattpocock/skills, `skills-lock.json`).
- Atualização automática do conteúdo embutido (sem auto-sync em runtime).
- Descoberta de skills remotas / marketplace.
- Execução paralela de runners (um `task` por vez, conforme limites da feature 010).
- Slash commands para skills não-kspec do projeto (skills de terceiros em `.agents/skills/` não viram comandos).
