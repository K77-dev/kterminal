# PRD — Squad mode (loop de engenharia multi-agente)

## Visão Geral

O kterminal hoje tem um único modo de trabalho: o fluxo SDD do kspec (brief → PRD → techspec → tasks → implement). Esse fluxo é ideal para features novas, mas pesado demais para spikes, refactors e bugs difíceis — problemas que exigem múltiplas perspectivas técnicas deliberando entre si antes de virar plano.

Este PRD introduz um segundo modo, o **squad mode**: um loop de engenharia multi-agente onde o agente principal assume o papel de **maestro**, convoca personas de disciplina (arquiteto, backend, frontend, banco, ux, qa, teste) que deliberam seguindo as rules, convergem num plano, e a execução desce para a maquinaria de subagentes existente. O roteador Jev escolhe o modelo de cada papel a cada chamada, pesando qualidade, custo e tokens/s reais medidos.

O valor: problemas que hoje exigem o usuário orquestrar manualmente várias perspectivas passam a ser tratados por uma mesa de especialistas automática, com roteamento de modelo por papel e custo controlado por tetos.

## Objetivos

- Oferecer um segundo modo de trabalho acionável num único comando (`/mode`), sem alterar o fluxo SDD default
- Tornar a deliberação transparente: mesa anunciada com papéis, plano e orçamento antes de qualquer gasto
- Rotear cada papel para o modelo adequado (raciocínio para arquiteto, rápido para qa), via Jev e tags do catálogo
- Controlar custo: tetos de convocações e tokens com defaults sensatos e override no kickoff
- Manter o modo persistente entre sessões (resume) e auditável no transcript (campo `agent` no JSONL)
- Dogfooding: a primeira mesa real (arquiteto + backend + qa) resolve um problema do próprio kterminal usando as rules Go

Métricas de sucesso: mesa converge sem intervenção dentro dos tetos; contribuições rastreáveis por papel no transcript; nenhum regresso no fluxo SDD (default intacto).

## Histórias de Usuário

Usuário primário: **desenvolvedor** que usa o kterminal no dia a dia. Secundário: **mantenedor** que configura personas, rules, pins e tetos do projeto.

- Como desenvolvedor, quero alternar para squad mode num comando, para atacar refactors e bugs difíceis sem forçá-los no fluxo SDD.
- Como desenvolvedor, quero ver a mesa anunciada (papéis, plano, orçamento estimado) antes das convocações, para saber o que vai acontecer e o custo previsto.
- Como desenvolvedor, quero enviar mensagens durante a deliberação, para redirecionar a mesa sem abortar o turno.
- Como desenvolvedor, quero contribuições rotuladas por papel com cor própria, para acompanhar quem disse o quê.
- Como desenvolvedor, quero tetos de custo automáticos, para que a mesa nunca gaste sem controle.
- Como mantenedor, quero definir personas em `.agents/agents/` que vençam as embutidas, para adaptar a mesa ao meu projeto.
- Como mantenedor, quero fixar (pin) o modelo de um papel no config.toml, para garantir previsibilidade de qualidade/custo.

## Funcionalidades Principais

### REQ-001 — Seletor de modo `/mode`

Popup na TUI com duas opções: `sdd` (default, fluxo de skills atual) e `squad`. O modo ativo aparece no hint bar, persiste no snapshot e é restaurado no resume. O default é configurável no config.toml.

#### Critérios de Aceite

- `/mode` abre popup navegável com `sdd` e `squad`; a seleção aplica imediatamente
- O modo ativo é visível no hint bar em ambos os modos
- O snapshot persiste o modo; o resume restaura `squad` se era o ativo ao salvar
- Sem configuração, o default é `sdd`; com configuração, o default do config.toml prevalece na abertura

---

### REQ-002 — Maestro prompt-driven

Ao ativar squad, o agente principal recebe o prompt de maestro (asset embutido, no estilo skill): convocar personas relevantes via tool `task`, passar o contexto acumulado da mesa, sintetizar contribuições e declarar convergência. Sem novo loop em Go.

#### Critérios de Aceite

- O prompt de maestro é embarcado como asset (`go:embed`)
- O maestro convoca personas exclusivamente via tool `task` existente
- O maestro sintetiza contribuições entre rodadas e declara convergência com o plano final
- Nenhum novo loop agêntico em Go (reuso do loop existente)

---

### REQ-003 — Catálogo de personas

7 personas default embutidas via `go:embed`: arquiteto, backend, frontend, banco, ux, qa, teste. Persona definida no projeto (`.agents/agents/<nome>/AGENT.md`) sobrescreve a embutida de mesmo nome. Frontmatter estruturado: `name`, `discipline`, `model-tags`, `rules`; o corpo contém o prompt do papel.

#### Critérios de Aceite

- As 7 personas embarcam no binário
- O frontmatter (`name`, `discipline`, `model-tags`, `rules`) é parseado pelo loader de personas
- Persona do projeto com mesmo nome sobrescreve a embutida
- Persona do projeto inédita é utilizável sem configuração adicional

---

### REQ-004 — Convocação dinâmica e subset de rules

O maestro monta a mesa por relevância ao pedido — num projeto Go, frontend/ux não são convocados. Cada convocation é um subagente com system prompt = persona + subset de rules da disciplina (fim do bundle-all para subagentes de squad). O mapeamento rule → disciplina usa o frontmatter (`paths`/`description`) das rules. Rules Go mínimas (build/vet/test/sem comentários) fazem parte do dogfooding.

#### Critérios de Aceite

- A mesa contém apenas papéis relevantes ao pedido
- O system prompt da persona inclui somente o subset de rules da disciplina (não o bundle completo)
- Rules Go mínimas estão disponíveis para consumo das personas

---

### REQ-005 — Roteamento por papel e pin manual

O papel entra no `state` da decisão de rota da persona; `Model.Tags` passa a integrar os critérios enviados ao Jev (ex.: arquiteto → tag `reasoning`, qa → tag `fast`). Pin manual por papel via config.toml (`[squad.pins]` papel → modelo) tem precedência sobre o roteamento.

#### Critérios de Aceite

- Persona com `model-tags: [reasoning]` roteia para modelo diferente do maestro (verificado com mock de gateway)
- Pin no config.toml fixa o modelo do papel, ignorando o roteador
- Sem pins configurados, o roteamento é feito pelo Jev com fallback heurístico

---

### REQ-006 — Kickoff, tetos e critério de saída

Antes da primeira convocation, o maestro anuncia a mesa proposta, o plano de rodadas e o orçamento estimado — e segue direto para as convocações, sem gate de aprovação. Limites no config.toml: `max_convocations` (default 8) e `token_budget` (default 200.000 tokens), com override declarado no kickoff. Fim da mesa: critério de saída declarado no kickoff, teto estourado, ou interrupção do usuário (Esc aborta o turno inteiro).

#### Critérios de Aceite

- O kickoff anuncia papéis, plano e orçamento antes da primeira convocation
- Defaults de 8 convocações / 200k tokens aplicam-se sem configuração
- A mesa encerra ao atingir o critério de saída, estourar teto ou receber Esc
- Override de tetos declarado no kickoff prevalece sobre os defaults do config

---

### REQ-007 — Fila de mensagens do usuário

Mensagens digitadas durante a mesa são enfileiradas e consumidas pelo maestro entre convocações. Esc continua abortando o turno inteiro.

#### Critérios de Aceite

- Mensagem enviada mid-mesa não interrompe a convocation em curso
- Mensagens enfileiradas são consumidas pelo maestro entre convocações (fila drenada entre steps)
- Esc aborta o turno inteiro, descartando a mesa em curso

---

### REQ-008 — Eventos, transcript e identidade visual por papel

Campo `Agent string` (aditivo, `omitempty`) em `agent.Event` e `session.Event`. A TUI renderiza cada contribuição com rótulo `▸ <papel>:` e cor por disciplina (paleta do tema). O transcript JSONL registra o campo para a conversa completa.

#### Critérios de Aceite

- Eventos de persona carregam o campo `agent` com o papel
- O JSONL registra `agent` em cada evento de persona
- A TUI renderiza `▸ <papel>:` com a cor da disciplina
- A cor nunca é o único indicador do papel (rótulo textual sempre presente)

---

### REQ-009 — Ponte SDD e execução do plano

A mesa lê e grava artefatos `spec/` via tools fs quando existirem. Após a convergência, o plano desce para execução via `task` (maquinaria existente). Personas recebem tools somente-leitura durante a deliberação; escrita acontece apenas na fase de execução.

#### Critérios de Aceite

- A mesa consulta artefatos `spec/` existentes via tools fs
- O plano convergido é entregue à execução via tool `task`
- Personas não escrevem arquivos nem executam comandos de escrita durante a deliberação
- Dogfooding: a primeira mesa real (arquiteto + backend + qa) resolve um problema do próprio kterminal usando as rules Go

## Experiência do Usuário

**Fluxo principal**: `/mode` → `squad` → pedido do usuário → kickoff (anúncio de mesa, plano, orçamento) → convocações sequenciais com contribuições rotuladas → convergência → plano → execução via `task`.

**Considerações de UI/UX**:

- Popup `/mode` segue o padrão do popup de comandos (navegação por setas, Enter confirma, Esc cancela)
- Modo ativo sempre visível no hint bar
- Contribuições renderizadas como `▸ <papel>:` com cor da disciplina na paleta do tema
- Mensagem digitada durante a mesa dá feedback de enfileiramento (não desaparece)

**Acessibilidade**:

- Rótulo textual sempre presente; cor nunca é o único indicador de papel
- Cores da paleta do tema com contraste adequado ao fundo do terminal
- Navegação do popup 100% por teclado, sem dependência de mouse

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo único `kterminal`, binário auto-contido
- Assets embutidos via `go:embed` (precedentes: `models.yaml`, `markdown.json`)
- Maestro prompt-driven: nenhum novo loop agêntico em Go; reuso de `newSubagent()`, `decide()` e da tool `task`
- Roteamento via Jev (Typesafe) com fallback heurístico; `Model.Tags` do catálogo integra os critérios de rota
- Config TOML em XDG (`~/.config/kterminal/config.toml`)
- v1 estritamente sequencial
- Código em inglês, sem comentários; specs em pt-BR

## Fora de Escopo

- Paralelismo de convocações (mesmo read-only) — fast-follow, junto com a UI de progresso múltiplo prevista em `10-subagentes.md`
- Pipeline SDD automatizado/gerenciado (`/mode sdd` automatizado)
- Enforcement hard de rules (validação mecânica de aderência)
- Personas editando código — simultâneo ou durante a deliberação
- Gate de aprovação interativo do kickoff
- Pin de modelo por comando em sessão (somente via config.toml)
- UI de progresso múltiplo
