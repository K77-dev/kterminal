# kterminal

> Um coding agent de terminal em Go, no estilo Claude Code/opencode — com uma mesa de engenharia multi-agente (o **squad mode**) e roteamento de modelo por chamada via engines selecionáveis (**Jev** ou **Laya**).

```text
$ go run .
▸ maestro: mesa registrada — roles=[architect backend qa] · 8 convocações · orçamento consultivo
```

## Sumário

- [O que é o kterminal](#o-que-é-o-kterminal)
- [Dois modos de trabalho](#dois-modos-de-trabalho)
- [Squad mode: a mesa de engenharia](#squad-mode-a-mesa-de-engenharia)
  - [A metáfora em uma frase](#a-metáfora-em-uma-frase)
  - [O elenco](#o-elenco)
  - [A vida de uma mesa](#a-vida-de-uma-mesa)
  - [Anatomia de uma rodada](#anatomia-de-uma-rodada)
  - [O que você vê na TUI](#o-que-você-vê-na-tui)
  - [O que cada persona vê (e o que não vê)](#o-que-cada-persona-vê-e-o-que-não-vê)
  - [Convergência e execução](#convergência-e-execução)
  - [Orçamento e limites](#orçamento-e-limites)
  - [Configuração](#configuração)
  - [Personas próprias](#personas-próprias)
  - [FAQ — as perguntas que todo mundo faz](#faq--as-perguntas-que-todo-mundo-faz)
- [Roteamento de modelo](#roteamento-de-modelo)
- [Processos em background](#processos-em-background)
- [rtk (compressão de output)](#rtk-compressão-de-output)
- [MCP Servers](#mcp-servers)
- [Sob o capô](#sob-o-capô)
- [Comandos](#comandos)
- [Solução de problemas](#solução-de-problemas)
- [Desenvolvimento](#desenvolvimento)

---

## O que é o kterminal

Um binário único em Go com uma TUI (Charmbracelet/Bubble Tea) que conversa com LLMs via cliente HTTP próprio (OpenAI-compatible, streaming). O diferencial duplo:

1. **Roteamento dinâmico** com engine selecionável: o **Jev** (roteador externo, Typesafe systemone) ou a **Laya** (decision engine local) escolhem o modelo a **cada chamada**, pesando qualidade, custo e tokens/s **reais medidos** — com fallback heurístico local.
2. **Squad mode**: em vez de um agente solitário, uma *mesa* de personas de engenharia que deliberam entre si até convergir num plano — e só então o plano desce para execução.

## Dois modos de trabalho

| | `/mode sdd` (padrão) | `/mode squad` |
|---|---|---|
| Quando usar | Features novas com spec completa | Spikes, refactors, bugs difíceis, decisões de design |
| Como funciona | Fluxo de skills: brief → PRD → techspec → tasks → implement | Mesa de personas delibera → plano → execução |
| Quem pensa | 1 agente seguindo a skill | Maestro + personas de disciplina |
| Saída | Artefatos `spec/` + código | Plano numerado + código |

O modo ativo aparece no hint bar, persiste no snapshot e é restaurado no `resume`. O padrão é configurável (`squad.default_mode`).

---

## Squad mode: a mesa de engenharia

### A metáfora em uma frase

Imagine uma **reunião de projeto**: um facilitador (o **maestro**) convoca especialistas (as **personas**), **um de cada vez**, resume o que ouviu e repassa ao próximo — até o grupo convergir num plano acionável.

Não é um chat em grupo onde todos falam juntos. É uma roda de conversa **conduzida**: quem faz a ponte entre os especialistas é sempre o maestro.

### O elenco

O maestro é o seu agente principal com outro chapéu: ao ativar `/mode squad`, o system prompt dele é trocado pelo [prompt de maestro](internal/squad/embed/maestro.md). As personas são subagentes especializados, cada um com prompt da disciplina, subset de rules e rota de modelo próprios:

| Persona | Disciplina | Model tags | Rules | Papel na mesa |
|---|---|---|---|---|
| `architect` | architecture | `reasoning` | go, code-standards | Fronteiras de módulos, acoplamento, seams |
| `backend` | backend | `reasoning`, `code` | go, code-standards | Lógica, loops, concorrência, erros |
| `frontend` | frontend | `code` | code-standards | UI, estados, rendering |
| `database` | database | `reasoning` | database | Esquemas, migrações, consultas |
| `ux` | ux | `reasoning` | — | Fluxos, ergonomia, clareza |
| `qa` | qa | `fast` | tests | Critérios de aceite, riscos, cobertura |
| `test` | test | `fast`, `code` | tests, go | Estratégia e casos de teste |

O maestro monta a mesa **por relevância**: num projeto Go de backend, `frontend` e `ux` simplesmente não são convocados.

### A vida de uma mesa

```mermaid
flowchart TD
    A["/mode squad"] --> B["Maestro assume"]
    B --> C["squad_kickoff<br>roles · max_convocations · token_budget · exit_criterion"]
    C --> D["task com persona<br>convoca a próxima persona da mesa"]
    D --> E["Persona delibera<br>read-only · rules da disciplina · rota própria"]
    E --> F["Contribuição + status<br>[mesa: N/M convocations · X/Y tokens]"]
    F --> G["Síntese do maestro<br>acordos · conflitos · dúvidas abertas"]
    G --> H{"Convergiu?<br>critério de saída · consenso · teto de convocações"}
    H -- "não" --> D
    H -- "sim" --> I["Plano final numerado"]
    I --> J["Execução via task<br>subagentes com poder de escrita"]
```

Passo a passo:

1. **Kickoff** — antes de convocar qualquer pessoa, o maestro registra a mesa com `squad_kickoff`: papéis, teto de convocações, orçamento de tokens e **critério de saída** (ex.: *"plano aprovado por todas as disciplinas"*). Sem kickoff, a convocation de persona falha.
2. **Convocation** — o maestro chama `task` com `persona: "<nome>"`, passando no `guidance` o contexto acumulado da mesa. Nasce um subagente isolado com o prompt da disciplina.
3. **Contribuição** — a persona devolve sua análise + uma linha de status `[mesa: 2/8 convocations · 30.4k/200k tokens]` para todos acompanharem o custo.
4. **Síntese** — entre convocações, o maestro produz um bloco de síntese (acordos, conflitos, questões abertas) e drena a fila de mensagens do usuário (ver [FAQ](#faq--as-perguntas-que-todo-mundo-faz)).
5. **Convergência** — a mesa encerra por critério de saída, consenso ou teto de convocações.
6. **Execução** — o plano numerado desce para subagentes de execução (com escrita) via `task`.

### Anatomia de uma rodada

```mermaid
sequenceDiagram
    participant U as Usuário
    participant MA as Maestro
    participant AR as architect
    participant BA as backend
    participant QA as qa

    U->>MA: "refatorar o loop agêntico"
    MA->>MA: squad_kickoff(roles=[architect, backend, qa], exit_criterion="plano aprovado")
    MA->>AR: task(persona="architect", guidance="problema + contexto")
    AR-->>MA: análise estrutural + [mesa: 1/8 · 12.3k/200k]
    MA->>BA: task(persona="backend", guidance="síntese + dúvidas abertas")
    BA-->>MA: proposta de implementação + [mesa: 2/8 · 30.4k/200k]
    MA->>QA: task(persona="qa", guidance="plano proposto até aqui")
    QA-->>MA: ressalvas de teste + [mesa: 3/8 · 41.8k/200k]
    MA-->>U: síntese final + plano numerado
    MA->>MA: task(execução — subagentes com escrita)
```

Repare: **as personas nunca se falam diretamente**. Cada seta entre personas passa pelo maestro, que decide o que repassar.

### O que você vê na TUI

Cada contribuição aparece no transcript com rótulo `▸ <papel>:` e cor da disciplina. Em terminais largos (≥ 100 colunas), uma sidebar mostra a mesa ao vivo:

```text
│ squad                                        │ ▸ maestro: mesa registrada — roles=[architect
│ ▸ maestro · deliberating                     │   backend qa] · critério de saída: plano aprovado
│   glm-5.3                                    │
│ ────────────────────────────                 │ ▸ architect: o runLoop mistura decisão de rota
│ ▸ architect · done                           │   e emissão de eventos; proponho extrair o
│   deepseek-v4.1 · 12.3k tok · $0.02          │   watchdog para um tipo próprio...
│   lendo internal/agent/agent.go              │   [mesa: 1/8 convocations · 12.3k/200k tokens]
│ ▸ backend · deliberating                     │
│   glm-5.3 · 18.1k tok · $0.03                │ ▸ backend: concordo com a fronteira, mas o
│   propondo refator do runLoop                │   cancel precisa ser por persona...
│ ▸ qa · waiting                               │   [mesa: 2/8 convocations · 30.4k/200k tokens]
│   —                                          │
│ ────────────────────────────                 │ ▸ maestro: ## Round 2 Synthesis
│ 2/8 convocations                             │   - Acordos: watchdog isolado
│ 30.4k/200k tokens                            │   - Conflitos: escopo do cancel
│                                              │   - Open questions: abort mid-convocation
│                                              │   - Next persona: qa — reason: risco de teste
```

O status de cada persona na sidebar conta a história: `waiting` → `deliberating` → `done`.

### O que cada persona vê (e o que não vê)

<details>
<summary><strong>Clique para expandir: o isolamento de cada persona</strong></summary>

Cada convocation cria um subagente **recém-nascido**, com memória zerada:

- **Vê**: seu system prompt (persona + apenas as rules da sua disciplina), a `description` e o `guidance` que o maestro escolheu repassar, e ferramentas de leitura (`read`, `glob`, `grep` — mais a tool `graphify` de consulta ao knowledge graph, quando `graphify-out/graph.json` existe) para inspecionar o código.
- **Não vê**: a conversa das outras personas, o histórico completo da sessão, ferramentas de escrita (`write`/`edit`/`bash` são bloqueados na mesa) e a tool `task` (personas não convocam personas — sem recursão).

É **hub-and-spoke**: o maestro é o único ponto de passagem. Isso tem três consequências práticas:

1. **Contexto cirúrgico** — cada persona recebe só o que importa para a disciplina dela (o maestro é instruído a não fazer bundle do contexto inteiro).
2. **Sem efeito cascata** — uma persona não contaminou a outra; divergências chegam ao maestro, que as explicita na síntese.
3. **Rota independente** — cada persona tem seu próprio critério na engine ativa: `architect` pede modelos `reasoning`, `qa` pede `fast`. A mesa inteira não paga o preço do modelo mais caro.

</details>

### Convergência e execução

A mesa termina por um destes caminhos:

| Gatilho | Tipo |
|---|---|
| `exit_criterion` declarado no kickoff foi atingido | Convergência |
| Todas as personas concordam com o plano | Convergência |
| Teto de convocações (`max_convocations`) esgotado | Teto duro |
| Usuário pressiona `Esc` | Aborto do turno inteiro |

O orçamento de tokens **não** encerra a mesa — veja [abaixo](#orçamento-e-limites).

Na convergência, o maestro entrega um plano com: problema (um parágrafo), decisões de design (bullets), passos de implementação (numerados, cada um mapeando arquivo/tipo), plano de teste (tabela de casos) e riscos/mitigações. Então chama `task` **sem** persona — que cria subagentes de execução com o registry completo (`read`, `glob`, `grep`, `write`, `edit`, `bash`). A mesa delibera; os subagentes executam — e isso é garantido mecanicamente: o toolset do maestro exclui `write`/`edit`/`bash` (fica com leitura, `ask_user`, `task` e `squad_kickoff`), então codificar direto não é uma opção nem se o modelo tentar.

### Orçamento e limites

| Controle | Natureza | Comportamento |
|---|---|---|
| `token_budget` | **Consultivo** | Aparece no status (`30.4k/200k tokens`, em cor de aviso quando estourado) apenas para informar. **Nunca bloqueia, nunca encerra a mesa.** O maestro pode até declarar um orçamento maior que o configurado — o valor declarado é respeitado. |
| `max_convocations` | **Duro** | É o guarda-chuva real: atingido o teto, a próxima convocation falha com erro e o maestro é obrigado a convergir. |
| Passos por subagente | **Duro** | 20 steps por subagente (persona ou executor). |
| Stall watchdog | **Duro** | 10 min sem progresso aborta o subagente com o parcial renderizado. |
| Profundidade | **Duro** | Subagentes não ganham a tool `task` — sem squads recursivas. |

### Configuração

```toml
# ~/.config/kterminal/config.toml
[router]
engine  = "jev"     # "jev" (padrão) ou "laya"
enabled = true      # false = modelo fixo (picker)

[squad]
default_mode     = "squad"   # "sdd" (padrão) ou "squad"
max_convocations = 8         # teto duro de convocações por turno
token_budget     = 200000    # orçamento consultivo — informativo, nunca bloqueia

[squad.pins]
architect = "glm-5.3"        # fixa o modelo de uma persona (bypassa o roteador)
```

- `[router] engine`: quem decide o modelo a cada chamada — `jev` (API Typesafe, exige api key) ou `laya` (local).
- `[router] enabled`: `false` desliga o roteamento dinâmico e usa o `model` fixo do topo da config.
- `default_mode`: modo inicial de toda sessão.
- `max_convocations`: máximo **configurável**; o maestro pode declarar menos no kickoff, nunca mais.
- `token_budget`: valor **default** quando o kickoff não declara um; dali em diante, só informativo.
- `pins`: override manual de rota por persona.

Overrides por env: `KTERMINAL_ROUTER` (engine), `KTERMINAL_MODEL` (modelo fixo), `KTERMINAL_TYPESAFE_ENABLED`, além dos de LLM (`KTERMINAL_LLM_BASE_URL`, `KTERMINAL_LLM_API_KEY`, timeouts e retries).

### Personas próprias

O catálogo de 7 personas vem embutido no binário (`go:embed`), mas o projeto vence: crie `.agents/agents/<nome>/AGENT.md` —

```markdown
---
name: perf
discipline: performance
model-tags: [fast]
rules: [go, code-standards]
---
You are the **perf** persona in a kterminal engineering squad.

Sua disciplina é **performance**. Você raciona sobre hot paths,
alocações, complexidade e custo de algoritmos...
```

- Uma persona do projeto com o mesmo **nome** de uma embutida a **substitui**.
- `rules` lista quais rules (de `.agents/rules/`) injetar no prompt — a persona não recebe o bundle inteiro, só o subset da disciplina.
- `model-tags` entra no critério de rota da engine ativa para aquela persona.

---

### FAQ — as perguntas que todo mundo faz

<details open>
<summary><strong>Uma persona conversa com a outra?</strong></summary>

**Não diretamente.** É hub-and-spoke: a persona nunca vê a conversa da outra — só recebe o contexto que o maestro repassa no `guidance`. Não é um chat em grupo; é um facilitador fazendo a ponte, rodada a rodada.

</details>

<details open>
<summary><strong>Uma ajuda a outra?</strong></summary>

**Sim, indiretamente.** A contribuição de uma vira contexto da próxima via síntese do maestro. E a mesa é dinâmica: se `architect` e `backend` concordam mas `qa` levanta uma dúvida, o maestro convoca `qa` em seguida. A ordem das convocações segue a relevância, não uma fila fixa.

</details>

<details open>
<summary><strong>Cada um faz um trabalho de cada vez?</strong></summary>

**Sim.** A v1 é estritamente sequencial: a `Mesa` registra os estados `waiting → deliberating → done` e só uma persona fica `deliberating` por vez. O `task` é síncrono — o maestro espera a persona terminar antes de sintetizar e convocar a próxima.

</details>

<details open>
<summary><strong>Duas ou mais personas podem trabalhar ao mesmo tempo?</strong></summary>

**Não na v1.** Paralelismo read-only (convocações sem tools de escrita) é fast-follow previsto no [brief](.docs/12-squad-multi-agente.md), junto com a UI de progresso múltiplo. Por ora: uma rodada, uma voz.

</details>

<details open>
<summary><strong>E se eu digitar no meio da mesa?</strong></summary>

Sua mensagem entra numa **fila** e o maestro a consome entre convocações, incorporando-a ao contexto da próxima. `Esc` continua abortando o turno inteiro (a mesa em progresso é descartada).

</details>

<details open>
<summary><strong>Personas podem escrever código?</strong></summary>

**Na mesa, não.** Personas recebem apenas `read`/`glob`/`grep` e a tool de consulta `graphify` — a mesa delibera sobre o código, não o altera. Quem escreve são os subagentes de execução, na fase pós-convergência, com o registry completo.

</details>

<details open>
<summary><strong>Se eu fechar o terminal, perco a mesa?</strong></summary>

Não. O modo e a mesa (papéis, statuses, consumo por persona) persistem no snapshot e são restaurados no `resume`. O transcript JSONL registra o campo `agent` em cada evento — a conversa completa fica auditável.

</details>

---

## Roteamento de modelo

O kterminal decide o modelo **a cada chamada** — não uma vez por sessão. A engine de decisão é selecionável em runtime:

| Engine | Onde roda | Requer | Comando |
|---|---|---|---|
| `jev` (padrão) | API externa Typesafe | `typesafe.api_key` | `/router jev` |
| `laya` | Local, in-process (ONNX) | artefatos em `~/.config/kterminal/laya/` | `/router laya` |
| *(desligado)* | — | — | `/router off` → picker de modelo fixo |

```text
$ kterminal --doctor
  router engine: jev
  routing enabled: yes
  model: auto
```

- `/router` ou `/router status` — mostra engine ativa, on/off e modelo vigente.
- `/router on|off` — liga/desliga o roteamento dinâmico; `off` abre o picker de modelo fixo.
- `/model <nome>` — pina um modelo manualmente (persiste na config, aparece como `⚑` no hint bar) · `/model auto` devolve a decisão à engine.
- Env override: `KTERMINAL_ROUTER=laya` (mesmo padrão dos demais overrides).

O hint bar mostra o estado: modelo atual (com `⚑` quando pinado), custo da sessão, tokens/s medidos, contador `bg N`, `plan` quando a camada de permissão está em plan, e `jev off`/`laya off` quando o roteamento está desligado.

## Processos em background

Comandos longos (dev servers, watchers, builds) não prendem mais o agente:

```json
{ "command": "bun run dev", "run_in_background": true }
```

A tool `bash` retorna imediato e o processo segue rodando em grupo próprio (grace de 250ms para falhas de startup). O rodapé mostra `bg N`; o popup **`/bg`** lista os processos com uptime e permite parar cada um individualmente (`↑↓` move, `enter` para, `esc` fecha). Na saída do kterminal — normal, erro ou SIGTERM/SIGINT — todos recebem TERM→KILL com budget de join.

---

## rtk (compressão de output)

O **rtk** (Rust Token Killer) é um CLI externo que reescreve comandos shell para comprimir a saída, reduzindo o consumo de tokens do LLM em 60–90%. Quando instalado e habilitado, toda chamada da tool `bash` passa por `rtk rewrite "<cmd>"` antes da execução — o comando reescrito vai ao shell e o output comprimido volta ao agente.

```toml
# ~/.config/kterminal/config.toml
[rtk]
enabled = true   # padrão; false desliga o rewrite
```

- **Detecção automática**: se o binário `rtk` não está no PATH, o agente funciona normalmente sem rewrite.
- **Comandos simples apenas**: pipes, redirecionamento, substitution e heredocs não são reescritos — vão direto ao shell.
- **Best-effort**: se `rtk rewrite` falhar ou timeout (3s), o comando original é executado.
- **Propagação**: subagentes (personas do squad e executores) herdam o rtk automaticamente.
- Env override: `KTERMINAL_RTK_ENABLED=false`.
- `/rtk` ou `/rtk status` mostra o estado; `/rtk on` / `/rtk off` toggle e persistem (reinicie para aplicar).

Instalação: `brew install rtk` (ou ver https://github.com/rtk-ai/rtk).

---

## MCP Servers

O kterminal suporta **MCP (Model Context Protocol)** servers via stdio. Servidores externos expõem tools que o agente pode invocar como qualquer outra tool do registry. Qualquer servidor MCP compatível funciona.

```toml
# ~/.config/kterminal/config.toml
[mcp]
enabled = true

[[mcp.servers]]
name = "filesystem"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
timeout = "30s"

[[mcp.servers]]
name = "github"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]
env = { GITHUB_TOKEN = "ghp_..." }
read_only = true
timeout = "45s"
```

| Campo | Descrição | Default |
|---|---|---|
| `name` | Identificador único do servidor (obrigatório) | — |
| `command` | Executável do servidor | — |
| `args` | Argumentos do comando | `[]` |
| `env` | Variáveis de ambiente | `{}` |
| `timeout` | Timeout por chamada | `"30s"` |
| `read_only` | Tools não-mutating (disponível em plan mode e personas) | `false` |

Comportamentos-chave:

- **Namespacing**: tools são registradas como `mcp__<server>__<tool>` para evitar colisão com tools built-in.
- **Permissões**: por default, MCP tools são tratadas como mutating — disparam confirmation prompts em `--confirm` e são bloqueadas em plan mode. `read_only = true` marca todas as tools do servidor como non-mutating — ficam disponíveis em plan mode e são injetadas em personas do squad.
- **Startup paralelo**: todos os servidores spawnam no boot. Falha = skip com warning; o agente funciona sem essas tools.
- **Shutdown graceful**: SIGTERM → 5s timeout → SIGKILL no process group inteiro.
- **Env override**: `KTERMINAL_MCP_ENABLED=false` desabilita todos os servidores.
- `kterminal --doctor` mostra o estado de cada servidor configurado.

---

## Sob o capô

<details>
<summary><strong>Mapa do código</strong></summary>

**Squad mode**

| Caminho | Responsabilidade |
|---|---|
| `internal/squad/squad.go` | Store de personas: catálogo embutido + override do projeto, frontmatter, resolução por nome |
| `internal/squad/kickoff.go` | `Kickoff`, `Limits` e validação (roles existem, convocações clampadas, orçamento default) |
| `internal/squad/mesa.go` | Estado da mesa: entries, statuses, contadores, status line thread-safe |
| `internal/squad/embed/maestro.md` | O prompt do maestro (protocolo de kickoff/convocation/convergência) |
| `internal/squad/embed/personas/*/AGENT.md` | As 7 personas embutidas |
| `internal/agent/agent.go` | `convokePersona`, `runSubagent` (watchdog, eventos), `task` com parâmetro `persona`, toolset restrito do maestro |
| `internal/tools/squad.go` | Tool `squad_kickoff` |
| `internal/tui/sidebar.go` | Sidebar da mesa (status, modelo, tokens, custo por persona) |

**Roteamento**

| Caminho | Responsabilidade |
|---|---|
| `internal/router/router.go` | Interface `Router`, `JevRouter`, `LayaRouter`, `HeuristicRouter`, `FixedRouter` |
| `internal/router/wire.go` | `WireWith` — factory que monta a engine ativa a partir da config |
| `internal/jev/` | Cliente HTTP do systemone (Typesafe) |
| `internal/laya/laya.go` | Porta da engine local: `Question`/`Answer`/`Status`, cache em `~/.config/kterminal/laya/` |
| `internal/agent/health.go` | Blocklist de modelos 404 (`modelHealth`), filtro de candidatos |
| `internal/telemetry/` | Tokens/s medidos que alimentam o critério de rota |

**Background e infra**

| Caminho | Responsabilidade |
|---|---|
| `internal/procmon/` | Manager de processos background: spawn em grupo próprio, watcher, `Stop` TERM→KILL, `KillAll` idempotente |
| `internal/tools/bash.go` | Tool `bash` com `run_in_background` e rewrite via rtk |
| `internal/rtk/rtk.go` | Detector do binário rtk (cache thread-safe) e `Rewrite` (best-effort, skip metacaracteres) |
| `internal/mcp/` | Cliente MCP (JSON-RPC sobre stdio), Manager, bridge para tools.Registry |
| `internal/tui/commands.go` | Comandos `/router`, `/model`, `/bg`, `/rtk`, popups, toggle plan/build (`tab`/`alt+tab`) |
| `internal/tools/graphify.go` | Tool read-only de consulta ao knowledge graph (`graphify-out/graph.json`) |

Decisões de projeto: sem novo loop em Go — o squad mode é **prompt-driven** sobre a maquinaria de subagentes existente; assets via `go:embed`; mesa thread-safe (`sync.Mutex`) porque os eventos chegam de goroutines de subagentes; engines de rota plugáveis atrás da interface `Router` (o wiring vive em `WireWith`, não no agente).

</details>

<details>
<summary><strong>Como o roteamento funciona</strong></summary>

A cada chamada, o `Router.Route(ctx, state, candidates)` decide o modelo. A engine ativa (`[router] engine`) determina quem decide:

- **Jev** (`JevRouter`) — API externa da Typesafe (`systemone`), exige `typesafe.api_key`. Pesando qualidade, custo e tokens/s reais (telemetria própria).
- **Laya** (`LayaRouter`) — decision engine local (ONNX in-process), sem api key. Faz a mesma pergunta tipada (`choice`) com os mesmos critérios do catálogo.
- **Fixed** (`FixedRouter`) — roteamento desligado (`/router off`): usa o modelo fixo da config.
- **Heuristic** (`HeuristicRouter`) — fallback local por keyword-matching quando a engine ativa falha.

No squad mode, o `state` carrega o papel da persona e suas `model-tags` entram no `Model.Criteria()` enviado à engine — `architect` → tag `reasoning` tende a modelos de raciocínio; `qa` → tag `fast` tende a modelos velozes. Um `pin` por persona (`[squad.pins]`) bypassa o roteador inteiro; `/model <nome>` pina o modelo da sessão principal (persistente).

Resiliência: um modelo que responde 404 no gateway entra na **blocklist** compartilhada (`modelHealth`) e sai dos candidatos até o fim do processo; em 429, o agente tenta os próximos modelos por probabilidade (`rateLimitFallbacks`) e o retry respeita `Retry-After` com piso de 15s.

</details>

<details>
<summary><strong>A ponte com o fluxo SDD</strong></summary>

A mesa não vive fora do processo do repo: personas leem artefatos de `spec/` com as tools de leitura quando existem, e o plano convergido pode alimentar um PRD/techspec. O desenvolvimento do próprio kterminal segue o fluxo kspec (brief → PRD → techspec → tasks) — o squad mode foi especificado em `.docs/12-squad-multi-agente.md` e dogfoodado com uma mesa real (architect + backend + qa) resolvendo um problema do próprio kterminal.

</details>

## Comandos

| Comando | Ação |
|---|---|
| `/mode` | Alterna entre `sdd` e `squad` (popup) |
| `tab` / `alt+tab` | Alterna entre `plan` (só pesquisa e planeja) e `build` (toolset completo) |
| `/router [status\|jev\|laya\|on\|off]` | Estado do roteamento ou troca de engine |
| `/model <nome>` | Pina um modelo (persistente) · `/model auto` devolve à engine |
| `/bg` | Popup de processos em background (parar individualmente) |
| `/rtk [status\|on\|off]` | Estado do rtk ou liga/desliga o rewrite de comandos |
| `/config` | Abre a configuração (gateway, keys, engine, modelo) |
| `/help` | Lista de comandos |
| `/clear`, `/image`, `/unimage`, `/exit` | Sessão e anexos |

`--doctor` executa checagens de setup (config, gateway, engine, catálogo, telemetria, **MCP servers**).

## Plan × Build

Camada de permissão independente do modo (estilo opencode): **plan** deixa o agente pesquisar (`read`/`glob`/`grep`), perguntar (`ask_user`) e entregar um plano — `write`, `edit`, `bash`, `kspec_bootstrap` e `task` ficam bloqueados (nem são oferecidos ao modelo). **build** devolve o toolset completo. A troca é instantânea com `tab` (input vazio) ou `alt+tab` (a qualquer momento), aparece no hint bar (`plan`) e persiste no snapshot do resume. Em squad mode o atalho é inerte — a mesa governa a si mesma e o aviso aponta para `/mode`.

## Solução de problemas

- **`reached max tool steps (50) without a final answer`** — o maestro rodou sem a disciplina da mesa: ou a sessão estava em `/mode sdd` (o padrão) quando a mesa foi aberta, ou o binário é anterior aos limites atuais. Confira com `/mode`; se mostrar `squad`, reconstrua o binário (`go build`). O modo não troca no meio do turno — defina-o **antes** do kickoff. Digitar `continue` retoma do snapshot com um novo orçamento de steps.
- Desde o fix, registrar a mesa com `squad_kickoff` ativa as restrições de maestro e o limite de 100 steps **em qualquer modo** — a linha de kickoff confirma com `maestro restrictions and 100-step limit now apply`.
- Personas têm teto **duro** de 20 steps e são instruídas a confiar nos fatos repassados pelo maestro em vez de reler arquivos. Se ainda assim uma persona bater no teto, o último texto escrito é resgatado: a contribuição chega com `partial contribution:`.
- O `guidance` repassado às personas é limitado: acima de ~6000 caracteres é truncado com o marcador `[guidance truncated]` (trecho inicial + conteúdo mais recente preservados).
- Timeout de stream: o first-byte timeout padrão agora é 120s (`llm.first_byte_timeout` no `config.toml`) — prompts muito grandes legitimamente demoram mais para produzir o primeiro token.

## Desenvolvimento

```bash
go build ./...                 # build
go run .                       # rodar em desenvolvimento
go run . --doctor              # checagens de setup (config, gateway, engine, catálogo, telemetria)
go run . --continue            # retomar a sessão mais recente
go run . --session <path>      # retomar um snapshot específico
go run . --confirm             # pedir confirmação antes de tools mutantes (write/edit/bash)
go vet ./...                   # análise estática
gofmt -l .                     # formatação (não deve listar nada)
go test ./...                  # testes
```

Verificação completa: `go build ./... && go vet ./... && gofmt -l . && go test ./...`

- Código-fonte em inglês, specs e documentação em português (Brasil).
- Sem comentários no código; tools registradas no construtor do `Registry`; assets via `go:embed`.
- Fluxo de entrega: branch de feature → fast-forward `develop` → `main` → push das três.
