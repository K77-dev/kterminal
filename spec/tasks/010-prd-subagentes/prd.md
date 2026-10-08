# PRD — Subagentes (tool `task`)

## Visão Geral

No kterminal, tarefas grandes serializam o agente principal: tudo acontece numa única conversa, com um único roteamento, e o contexto da tarefa principal vai se contaminando com detalhes de subtarefas. Delegar subtarefas a subagentes — cada um com mensagens próprias, roteamento Jev próprio por chamada e tools próprios — permite contexto limpo e segue o padrão `task` consolidado no Claude Code/opencode.

Este PRD define a tool `task(description)`: o agente principal delega uma subtarefa a um subagente isolado que a executa até o fim e devolve a resposta final como resultado da tool.

## Objetivos

- **Contexto limpo**: a conversa principal não se contamina com os passos intermediários da subtarefa.
- **Roteamento independente**: cada subagente tem seu próprio Jev escolhendo modelo a cada chamada.
- **Segurança operacional**: limites rígidos (profundidade, steps, timeout) garantem que um subagente problemático nunca derrube o agente principal.
- **Visibilidade**: a execução do subagente é visível na TUI como eventos aninhados, claramente distinguíveis do fluxo principal.

## Histórias de Usuário

- Como Jev, quero que o agente delegue subtarefas isoladas via `task`, para que a conversa principal fique limpa e focada.
- Como Jev, quero ver a execução do subagente (rotas, tools) indentada e diferenciada no chat, para acompanhar o que acontece sem confundir com o fluxo principal.
- Como Jev, quero que um subagente que estoura limites (steps/timeout) devolva erro como resultado sem derrubar o agente principal, para que o trabalho continue.
- Como Jev, quero que Esc cancele o turno inteiro incluindo subagentes ativos, para ter controle total do que roda.

## Funcionalidades Principais

### REQ-001 — Tool `task`

Nova tool `task` disponível ao agente principal:

- Schema: `description` (obrigatória) e `guidance` (opcional).
- É uma tool mutante: herda as regras de confirmação do modo `--confirm`.
- Disponível apenas no agente principal — subagentes não spawnam subagentes (limite de profundidade 1).

#### Critérios de Aceite

- O agente principal pode chamar `task`; o subagente não vê a tool nas suas definições.

---

### REQ-002 — Subagente isolado

O subagente roda com infraestrutura compartilhada (gateway LLM, router, fallback, catálogo, sessão, configuração de confirmação) mas estado próprio:

- Mensagens zeradas, iniciadas com prompt de sistema específico de subagente (conciso, retorna só o resultado final) + description + guidance.
- Roteamento Jev próprio a cada chamada.
- Limite de steps: 10 (metade do agente principal).
- O resultado final do subagente chega ao agente principal como resultado da tool.

#### Critérios de Aceite

- Subagente roda com roteamento próprio (modelo escolhido independente do principal).
- A resposta final do subagente chega como tool result e o principal continua com ela.

---

### REQ-003 — Eventos aninhados

Os eventos do subagente (rota, tool start, tool result) chegam à TUI marcados como aninhados (profundidade e tool de origem), permitindo distinguir o fluxo do subagente do fluxo principal.

#### Critérios de Aceite

- Eventos do subagente são renderizados como aninhados, não misturados ao fluxo principal.

---

### REQ-004 — Limites e timeout

- Timeout de 5 minutos por subagente; estourou → resultado é um erro explícito de timeout.
- Subagente que estoura 10 steps devolve erro como resultado.
- Em ambos os casos, o agente principal segue vivo e decide o próximo passo.

#### Critérios de Aceite

- Subagente que estoura 10 steps ou 5min devolve erro como resultado — o principal segue vivo.

---

### REQ-005 — Cancelamento compartilhado

O cancelamento do turno (Esc) cancela também os subagentes ativos — o contexto do turno é compartilhado.

#### Critérios de Aceite

- Esc cancela o turno inteiro, incluindo subagentes em execução.

---

### REQ-006 — TUI e transcript

- TUI: eventos aninhados renderizam com indentação de 2 espaços e cor secundária (azul); o hint bar mostra `subagent running` enquanto há subagente ativo.
- Transcript: eventos aninhados gravados com campo de profundidade no JSONL.

#### Critérios de Aceite

- O fluxo do subagente é visualmente distinguível no chat e no transcript.

## Experiência do Usuário

- A delegação aparece como uma tool normal na conversa; o detalhe da execução fica indentado e em cor distinta — quem não quiser ler, ignora.
- O hint bar sinaliza quando um subagente está ativo, mantendo o usuário ciente do estado.
- A experiência líquida: tarefas grandes progridem com a conversa principal enxuta e legível.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- **Sem goroutines paralelas na v1** — `task` é sequencial. Requisito não negociável (paralelismo exige cancelamento e UI de progresso múltiplo primeiro).
- Limite de profundidade 1: subagentes não spawnam subagentes.
- Timeout de 5 minutos e máximo de 10 steps por subagente.

## Fora de Escopo

- Paralelismo real entre subagentes.
- Múltiplos subagentes simultâneos.
- Profundidade maior que 1 (subagentes de subagentes).
- Subagentes nomeados, persistidos ou reutilizáveis entre turnos.
- Progresso granular do subagente na UI (além dos eventos aninhados).
