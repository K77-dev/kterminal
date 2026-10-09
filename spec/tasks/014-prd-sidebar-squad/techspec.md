# Tech Spec — Sidebar de agentes do squad

## Requisitos Atendidos

- REQ-001 — Exibição condicionada ao modo squad
- REQ-002 — Painel da mesa
- REQ-003 — Atividade ao vivo
- REQ-004 — Métricas por persona e orçamento da mesa
- REQ-005 — Layout responsivo
- REQ-006 — Persistência e resume

## Resumo Executivo

O sidebar é uma coluna renderizada por `lipgloss.JoinHorizontal` à direita do chat, visível somente em modo squad com terminal ≥ 100 colunas, alimentada por um novo tipo `squad.Mesa` mantido pelo Agent. A `Mesa` é atualizada nos pontos existentes do loop agêntico (kickoff, convocation, acumulação de usage, fim de turno) e lida pela TUI via getter a cada render — nenhum novo loop, canal ou evento-kind. Eventos aninhados existentes (`tool_start`, `turn_done`) ganham campos aditivos com tokens/custo cumulativos do subagente, dando métricas por persona ao vivo durante a deliberação.

Decisões-chave: (1) estado da mesa é domínio do `internal/squad` e propriedade do Agent — snapshot e resume saem grátis, TUI permanece fina e sem fonte de verdade própria; (2) os contadores da Mesa espelham `turnTokens`/`turnConvocations` nos mesmos pontos de mutação, garantindo por construção que o rodapé bate com os valores que o enforcement de tetos usa; (3) atividade corrente é transiente na TUI (derivada de eventos aninhados), não persistida — o snapshot carrega personas, status e métricas, conforme REQ-006; (4) maestro aparece no topo com identificação, status e modelo, sem métricas próprias.

## Arquitetura do Sistema

### Visão Geral dos Componentes

**Novos:**

- `internal/squad/mesa.go` — tipo `Mesa` (estado persistível da mesa: roles do kickoff, tetos, contadores do turno, entradas por persona com status/modelo/tokens/custo) com mutex interno e métodos de transição.
- `internal/tui/sidebar.go` — render do painel: cálculo de largura, cabeçalho, linha do maestro, entradas por persona, rodapé de orçamento, estado "mesa não iniciada".

**Modificados:**

- `internal/agent` — campo `TurnTokens int64` em `Event` (aditivo); campo `mesa *squad.Mesa` + getters `Mesa()`/`RestoreMesa()`; espelhamento das mutações de `turnTokens`/`turnConvocations` na Mesa; transições de status em `RegisterKickoff`/`convokePersona`/`endTurn`; observação de métricas no drain de `runSubagent`; `writeSnapshot` passa a Mesa.
- `internal/session` — `Event.Mesa` e `Snapshot.Mesa *squad.Mesa` (aditivos, `omitempty`); `WriteSnapshot` ganha parâmetro.
- `internal/tui` — `View()` compõe chat + sidebar horizontalmente; `WindowSizeMsg` deriva larguras da coluna de chat; `handleAgentEvent` trata `EventKickoff` (hoje ignorado) e limpa atividade; `handleNestedEvent` mantém mapa transiente de atividade por persona; `hintBar` usa a largura do chat; estilos do sidebar em `theme.go`.
- `main.go` — resume: `ag.RestoreMesa(resumed.Mesa)` após restaurar o modo.

**Fluxo de dados**: `/mode squad` → sidebar surge com "mesa não iniciada" (`Mesa == nil`) → `squad_kickoff` → `RegisterKickoff` reconstrói a Mesa (roles + disciplinas resolvidas) → `EventKickoff` → TUI re-renderiza → convocation: `convokePersona` marca deliberando e conta; eventos aninhados enriquecidos atualizam modelo/tokens/custo da persona via drain → contribuição termina → status concluída → `endTurn` congela statuses → `writeSnapshot` persiste a Mesa → resume reconstrói o painel. `/mode sdd` → `View()` omite o sidebar e o chat volta à largura total.

## Design de Implementação

### Interfaces Principais

```go
type MesaEntry struct {
    Name       string  `json:"name"`
    Discipline string  `json:"discipline,omitempty"`
    Status     string  `json:"status"` // waiting | deliberating | done
    Model      string  `json:"model,omitempty"`
    Tokens     int64   `json:"tokens,omitempty"`
    Cost       float64 `json:"cost,omitempty"`
}

type Mesa struct {
    Roles           []string    `json:"roles,omitempty"`
    MaxConvocations int         `json:"max_convocations,omitempty"`
    TokenBudget     int64       `json:"token_budget,omitempty"`
    Convocations    int         `json:"convocations,omitempty"`
    Tokens          int64       `json:"tokens,omitempty"`
    Entries         []MesaEntry `json:"entries,omitempty"`
}
```

```go
func (m *Mesa) Reset(k Kickoff, disciplines map[string]string)
func (m *Mesa) ResetTurn()                    // zera Convocations/Tokens no início do turno
func (m *Mesa) AddTokens(n int64)             // espelha mutações de turnTokens
func (m *Mesa) AddConvocation()
func (m *Mesa) StartDeliberation(name string) // captura baseline da convocation
func (m *Mesa) FinishDeliberation(name string)
func (m *Mesa) FinishTurn()                   // deliberando → done no fim do turno
func (m *Mesa) ObservePersona(name, model string, tokens int64, cost float64) // drain
```

`ObservePersona` recebe os cumulativos do subagente (por convocation) e soma ao baseline capturado em `StartDeliberation` — a entrada acumula entre convocations da mesma persona. Todos os métodos são seguros para concorrência (mutex interno; `squad` não importa `agent`, params primitivos).

No Agent: `func (a *Agent) Mesa() *squad.Mesa` (cópia rasa sob lock para render) e `func (a *Agent) RestoreMesa(m *squad.Mesa)`. Em `Event`: `TurnTokens int64` — cumulativo do `runLoop` corrente, carimbado junto com `SessionCost` em `EventToolStart` e `EventTurnDone` após a acumulação de usage (agent.go:713-789); em depth 0 os campos são ignorados pela TUI hoje, sem regressão.

Na TUI: `func (m Model) sidebarWidth() int` (0 quando oculto) e `func (m Model) sidebarView(width int) string`.

### Modelos de Dados

- `session.Event` ganha `Mesa *squad.Mesa \`json:"mesa,omitempty"\`` — presente apenas na linha `snapshot`.
- `session.Snapshot` ganha `Mesa *squad.Mesa`; `WriteSnapshot(messages []llm.Message, skill, mode string, mesa *squad.Mesa)`. Snapshots antigos (sem `mesa`) desserializam com `Mesa == nil` → painel exibe "mesa não iniciada" (REQ-006).
- Atividade transiente na TUI: `personaActivity map[string]string` (persona → `tool(args)` truncado), atualizada em `handleNestedEvent`; limpa no `EventKickoff`. Não persiste: personas concluídas mostram a última ação da sessão corrente ou "—" se a persona deliberou sem tools.

### Endpoints de API

Não aplicável — funcionalidade integralmente local à TUI.

## Pontos de Integração

Não há novas integrações externas. Jev e gateway inalterados; os valores exibidos derivam do `Usage` já retornado pelo `ChatStream` e registrado no JSONL — o critério "sidebar bate com o transcript" é satisfeito por construção (mesmas mutações, mesmos valores).

## Verificações Técnicas

### Segurança

- Sidebar é somente leitura por natureza: string renderizada, sem foco, sem handler de tecla — impossível capturar input ou desviar atalhos.
- Nenhum dado sensível novo: modelos, tokens e custos já constam no JSONL; a Mesa não carrega prompts nem conteúdo de mensagens.
- Nenhuma tool nova; personas continuam com registry somente-leitura.

### Arquitetura

- **Igualdade por construção**: `Mesa.Tokens`/`Convocations` espelham `a.turnTokens`/`a.turnConvocations` nos mesmos pontos de mutação (início de turno, acumulação por chamada em depth 0, retorno de `runSubagent`, `convokePersona`) — o rodapé mostra exatamente o que o enforcement de tetos mede, incluindo a mesma defasagem mid-convocation (tokens do sub em voo só aterrissam no retorno de `runSubagent`, como hoje).
- **Concorrência**: Mesa acessada por 3 goroutines (runLoop, drain de subagent, reader da TUI) — mutex interno; testes com `-race`.
- **Ponto de falha**: divergência futura se alguém mutar `turnTokens` sem espelhar — mitigada por teste de igualdade pós-turno e por espelhos colados às mutações existentes.
- **Fim de turno**: `endTurn` (depth 0) chama `FinishTurn()` — persona deliberando em turno abortado vira `done`, atendendo "mesa encerrada reflete o estado final".

### Infraestrutura

- Binário auto-contido; zero dependências novas (lipgloss/bubbles já usados). `session` passa a importar `squad` (leaf, sem ciclo — mesmo padrão do import de `llm`).
- Rollback: `/mode sdd` remove o painel sem código condicional residual; snapshots com `mesa` são ignorados por binários antigos (campo JSON desconhecido).

## Abordagem de Testes

### Testes Unidade

- `internal/squad`: ciclo da Mesa (Reset com disciplinas, ResetTurn, AddTokens/AddConvocation, Start/FinishDeliberation, ObservePersona com baseline entre convocations, FinishTurn), round-trip JSON com `omitempty`, acesso concorrente sob `-race`.
- `internal/session`: round-trip `WriteSnapshot`→`Load` com Mesa; snapshot antigo sem `mesa` carrega com `Mesa == nil`.
- `internal/agent` (mocks existentes de `agent_test.go`): mesa completa com kickoff + 2 convocations — statuses transitem, `Event.ToolStart`/`TurnDone` aninhados carregam `TurnTokens`/`SessionCost` cumulativos, `Mesa.Tokens == turnTokens` ao fim do turno, snapshot contém a Mesa; abort mid-convocation → `FinishTurn` marca `done`; segunda convocation da mesma persona acumula sobre o baseline.
- `internal/tui`: matriz de visibilidade (sdd nunca; squad <100 cols ausente; ≥100 presente); clamp de largura (100→24, 140→35, 200→40); conteúdo com Mesa injetada via `RestoreMesa` (nome + status textual, cor nunca única; elisão com "…"; rodapé `n/max` e `k/budget`); "mesa não iniciada" com Mesa nil; atividade ao vivo via eventos aninhados; `EventKickoff` limpa atividade e re-renderiza; resize 80→120→200 mantém ambos os painéis sem overflow (`lipgloss.Width` da View == terminal); render em sdd byte-a-byte idêntico ao atual (zero regressão).

### Testes de Integração

- Resume: sessão em squad com mesa encerrada → `--continue` reconstrói personas/status/métricas no painel; snapshot pré-feature carrega e exibe "mesa não iniciada".
- Fluxo completo com mock gateway: kickoff → convocações → convergência → `/mode sdd` → `/mode squad` (mesa preservada) → novo kickoff reconstrói a Mesa.

### Testes de E2E

Sem TestSprite (TUI local, precedente da feature 012). E2E = dogfooding documentado: mesa real no próprio kterminal com resize ao vivo (80/120/200 cols), interrupção com Esc e resume, conferindo sidebar × transcript JSONL.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. `internal/squad/mesa.go` — tipo e transições, com testes. Fundação sem dependências.
2. `internal/session` — campos aditivos e assinatura de `WriteSnapshot` (quebra compile das chamadas, forçando o passo 3).
3. `internal/agent` — `Event.TurnTokens`, campo mesa, espelhos, transições, drain, getters, `RestoreMesa`, passagem ao snapshot.
4. `internal/tui/sidebar.go` + estilos — render puro lendo `Mesa()`, com testes de largura/conteúdo.
5. `internal/tui` integração — `View()`/`WindowSizeMsg`/`hintBar` com coluna de chat, casos de evento, atividade transiente; wiring de resume no `main.go`.
6. Regressão completa (`go build && go vet && gofmt -l . && go test ./... -race`) + dogfooding.

### Dependências Técnicas

- Nenhuma externa bloqueante. 2 depende de 1; 3 depende de 1–2; 4–5 dependem de 3; 6 fecha o ciclo.

## Monitoramento e Observabilidade

### Error Tracking

Sem ferramenta externa (CLI local). Erros do fluxo de mesa continuam fluindo pelo `EventError` existente → JSONL. O sidebar não introduz caminhos de erro próprios: Mesa nil é estado legítimo ("mesa não iniciada"), não falha.

### Logging Estruturado

O transcript JSONL permanece o log. A linha `snapshot` ganha `mesa` (roles, tetos, consumo, entradas) — rastreável no resume. Nenhum dado sensível novo; prompts e conteúdo de mensagens continuam fora da Mesa.

### Health Checks

Não aplicável (CLI). `--doctor` inalterado — a Mesa é estado de sessão, não de setup.

### Métricas de Negócio

O sidebar é ele mesmo a métrica em-app: convocações e tokens contra os tetos do kickoff, custo por persona, modelo roteado por papel — alimentando a avaliação do roteamento Jev já coberta pelo `telemetry.Store`.

### Alertas

Não aplicável (CLI local). Equivalente em-app: o rodapé exibe consumo contra teto em tempo real; estouro continua bloqueado mecanicamente pelo `convokePersona`.

## Considerações Técnicas

### Decisões Principais

- **`squad.Mesa` no Agent** em vez de derivação na TUI: uma fonte de verdade; snapshot/resume grátis; TUI só renderiza. Alternativa rejeitada: acumuladores duplicados (TUI para o vivo, Agent para o snapshot) divergem.
- **Espelhar contadores nos pontos de mutação** em vez de expor `turnTokens` por getter: o valor persistido e o exibido saem da mesma escrita; igualdade com o enforcement garantida por construção.
- **Campos aditivos em eventos existentes** (`TurnTokens`/`SessionCost` em `tool_start`/`turn_done` aninhados) em vez de novos event kinds: respeita a restrição do PRD (nenhum novo canal/loop) e dá granularidade por evento de persona; `SessionCost` já existe no struct — só passa a ser carimbado em aninhados.
- **Atividade transiente na TUI**: REQ-006 persiste "personas, status e métricas" — atividade corrente é efêmera por definição; persisti-la aumentaria o snapshot sem ganho (persona deliberando no abort vira `done`).
- **Snapshot tipado `Mesa *squad.Mesa`** em vez de `json.RawMessage`: `session` já importa tipos de domínio (`llm.Message`); `squad` é leaf sem ciclo; menos código de decodificação manual.
- **Maestro sem métricas próprias**: aderente a REQ-002/004 (identificação no topo; métricas por persona; total da mesa no rodapé); o custo do maestro já é visível no hint bar da sessão.
- **Estados modais (config/confirm/ask) continuam full-width**: popups e wizards existentes inalterados; o sidebar reaparece ao voltar ao chat.

### Riscos Conhecidos

- **Race no drain × runLoop × TUI**: mutex na Mesa + testes `-race`; a Mesa nunca é compartilhada sem lock (getter devolve cópia).
- **Divergência futura mesa × turnTokens**: teste de igualdade pós-turno; espelhos adjacentes às mutações.
- **Re-render do glamour em resize**: cache existente invalida por largura — mudanças de largura mais frequentes com sidebar, custo baixo (re-render sob demanda).
- **Persona sem tools** (contribuição em texto puro): atividade vazia → "—"; métricas chegam no `turn_done` aninhado.
- **Largura exata com ANSI**: usar `lipgloss.Width` nos testes; truncar por runes com `truncate` existente.

### Conformidade com Skills Padrões

`.agents/rules/architecture-ddd.md` orienta TypeScript/Java com `src/modules/`; o kterminal é Go flat com `internal/` — prevalecem os padrões do código-base (AGENTS.md); a seção Bounded Context do template não se aplica. Não há tabela "Stack e skills recomendadas" no CLAUDE.md; skills de fluxo para o próximo passo: `kspec-tasks` (decomposição) e `kspec-implement` (execução). Rules Go do código-base: sem comentários, eventos via canal existente, receivers por valor em `View`/render, `strings.Builder` por ponteiro, textos da TUI em inglês (status: `waiting`/`deliberating`/`done`), spec em pt-BR.

### Arquivos relevantes e dependentes

**Novos:**
- `internal/squad/mesa.go`, `internal/squad/mesa_test.go`
- `internal/tui/sidebar.go`, `internal/tui/sidebar_test.go`

**Modificados:**
- `internal/agent/agent.go` — `Event.TurnTokens` (agent.go:61), campo mesa (agent.go:121), `RegisterKickoff` (agent.go:215), drain de `runSubagent` (agent.go:383), `convokePersona` (agent.go:533), acumulações do `runLoop` (agent.go:713), `writeSnapshot` (agent.go:260), `endTurn` (agent.go:284)
- `internal/session/session.go` — `Event.Mesa` (session.go:21), `Snapshot` (session.go:92), `WriteSnapshot` (session.go:98), `Load` (session.go:152)
- `internal/tui/tui.go` — campos do Model (tui.go:75), `WindowSizeMsg` (tui.go:299), `handleAgentEvent` (tui.go:396), `handleNestedEvent` (tui.go:497), `hintBar` (tui.go:1383), `View` (tui.go:1442)
- `internal/tui/theme.go` — estilos do sidebar (borda/gutter `colBorder`, fundo `colBgPanel`, `disciplineColor` existente)
- `main.go` — `RestoreMesa` no resume (main.go:112)
- `internal/session/session_test.go`, `internal/agent/agent_test.go`, `internal/tui/tui_test.go` — testes
