# Tech Spec — Gestão de janela de contexto (compação)

## Requisitos Atendidos

- REQ-001 — Estimativa de tokens por chamada
- REQ-002 — Compação automática em 70% da janela
- REQ-003 — Truncamento de tool results gigantes
- REQ-004 — Evento de compação

## Resumo Executivo

O `Agent` passa a rastrear `lastPromptTokens` (medição real de `StreamResult.Usage.PromptTokens`) e, antes de cada chamada ao LLM, estima o custo do próximo prompt (medição + delta aproximado, com fallback heurístico `len/4`). Quando a estimativa excede 70% do `ContextWindow` do modelo escolhido, a compação executa antes da chamada: os turnos antigos (tudo exceto as últimas 4 mensagens) são enviados a um modelo barato fixo (`deepseek-v4.1-flash`, fallback para o default do catálogo) via `ChatStream` sem tools, pedindo um resumo denso; o histórico é substituído por `[mensagem role "system" com o resumo] + últimas 4 mensagens`. Se mesmo compactado o prompt não couber, tool results antigos são truncados para 2000 caracteres — o truncamento é a garantia dura de nunca estourar. Um novo `EventCompaction` (tokens antes/depois) chega à TUI (linha sutil muted) e ao transcript. A compação é mecânica: limiar de tokens, zero interação com o usuário.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/agent/agent.go`** (modificado — núcleo): campos `lastPromptTokens int64` e `lastEstimateChars int`; métodos `estimateTokens() int64`, `needsCompaction(model) bool`, `compact(ctx) error`, `truncateOldToolResults() bool`; novo `EventKind` `EventCompaction`; gravação do evento `compaction` no transcript.
- **`internal/llm/llm.go`** (sem mudança): `ChatStream` já retorna `Usage.PromptTokens`; a chamada de resumo reusa o mesmo método com `tools = nil`.
- **`internal/catalog/catalog.go`** (sem mudança): `Model.ContextWindow` já existe no YAML.
- **`internal/tui/tui.go`** (modificado): caso `EventCompaction` → linha `⚡ context compacted (12.4k → 3.1k tokens)` em `colTextMuted`, sem interromper a leitura.
- **`internal/session/session.go`** (modificado): `Event` ganha campos `TokensBefore/TokensAfter int64` (`omitempty`).

Fluxo de dados (dentro de cada step do loop, após `decide` e antes do `ChatStream`):

1. `estimateTokens()` = `lastPromptTokens + (charsAtuais - lastEstimateChars)/4` quando há medição; senão `len(mensagens concatenadas)/4`.
2. `estimate > 0.7 * ContextWindow(decision.Model)` → `compact(ctx)`.
3. `compact`: monta prompt de resumo com `messages[:len-4]`, chama `ChatStream` no modelo de resumo (sem tools), substitui `messages` por `[system resumo] + últimas 4`, emite `EventCompaction{TokensBefore, TokensAfter}` e grava no transcript.
4. Pós-compação, re-estima; ainda estourando → `truncateOldToolResults()` (loop até caber ou não haver mais o que truncar).

## Design de Implementação

### Interfaces Principais

```go
const compactionThreshold = 0.7
const preservedTailMessages = 4
const summaryModel = "deepseek-v4.1-flash"
const toolResultTruncateChars = 2000

func (a *Agent) estimateTokens() int64
func (a *Agent) needsCompaction(window int) bool
func (a *Agent) compact(ctx context.Context) error
func (a *Agent) truncateOldToolResults() bool
```

Estrutura da compação:

```go
func (a *Agent) compact(ctx context.Context) error {
	summaryInput := buildSummaryPrompt(a.messages[:len(a.messages)-preservedTailMessages])
	model := summaryModel
	if _, ok := a.Catalog.Get(model); !ok || !available(a.candidates, model) {
		model = a.Catalog.DefaultModel
	}
	res, err := a.LLM.ChatStream(ctx, model, []llm.Message{{Role: "user", Content: summaryInput}}, nil, nil)
	if err != nil {
		return err
	}
	before := a.estimateTokens()
	tail := append([]llm.Message{}, a.messages[len(a.messages)-preservedTailMessages:]...)
	a.messages = append([]llm.Message{{Role: "system", Content: res.Content}}, tail...)
	after := a.estimateTokens()
	a.emit(Event{Kind: EventCompaction, TokensBefore: before, TokensAfter: after})
	a.Session.Write(session.Event{Type: "compaction", TokensBefore: before, TokensAfter: after})
	return nil
}
```

Regras:

- `buildSummaryPrompt` serializa os turnos antigos como texto (role + content truncado por mensagem a 2000 chars) e pede: resumo denso com decisões, arquivos tocados, estado da tarefa.
- Falha da chamada de resumo → compação aborta silenciosamente e o fluxo cai no truncamento (REQ-003) — a garantia de não-estouro nunca depende do LLM de resumo funcionar.
- `truncateOldToolResults`: percorre `messages` da mais antiga para a mais recente; mensagens `role: "tool"` com `len(Content) > 2000` → `Content[:2000] + "… (truncated)"`; repete a estimativa após cada truncamento; para quando a estimativa cabe ou não truncou nada.
- `lastPromptTokens` atualizado a cada `StreamResult` recebido no loop principal (não na chamada de resumo).
- A estimativa por chars usa o mesmo concatenado que vira prompt (aproximação `/4` documentada no PRD como heurística).

### Modelos de Dados

```go
const EventCompaction EventKind = "compaction"
```

- `agent.Event`: novos campos `TokensBefore, TokensAfter int64` (usados só em `EventCompaction`).
- `session.Event`: novos campos `TokensBefore int64 \`json:"tokens_before,omitempty"\`` e `TokensAfter int64 \`json:"tokens_after,omitempty"\`` — eventos existentes permanecem byte-idênticos (`omitempty`).
- Primeira mensagem `role: "system"` do projeto (confirmado com o usuário): o resumo entra como system message — padrão OpenAI-compatible aceito pelo gateway.

## Pontos de Integração

- **Gateway LLM**: chamada de resumo é um `ChatStream` comum (`stream: true`, sem `tools`) — nenhum endpoint novo; o modelo de resumo precisa estar disponível no gateway (`ensureCandidates` já validou a interseção; fallback para `Catalog.DefaultModel` cobre indisponibilidade).
- **Jev/router**: fora do fluxo — a compação acontece após o `decide`, não consome roteamento (o modelo de resumo é fixo, não roteado).

## Verificações Técnicas

### Segurança

- O prompt de resumo envia o conteúdo da conversa ao mesmo gateway que já recebe o histórico inteiro hoje — nenhum dado novo sai do ambiente.
- Sem validação de input nova; limiares são constantes do código.

### Arquitetura

- Compação e truncamento são métodos privados do `Agent` — ponto único de decisão; TUI e session apenas consomem eventos.
- Ordem no loop: `decide` → `needsCompaction(window do modelo decidido)` → `compact` → re-estimar → `truncate` → `ChatStream`. O modelo decidido pelo Jev define a janela — compação ocorre após o roteamento, evitando compactar para janela errada.
- Invariantes: últimas 4 mensagens nunca compactadas nem truncadas (o truncamento para antes da cauda preservada).
- `emit` não-bloqueante preservado; `EventCompaction` segue o mesmo canal (buffer 512).

### Infraestrutura

- Sem novos requisitos — stdlib. Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/agent/agent_test.go`**, com catálogo de teste de janela pequena (ex.: `ContextWindow: 2000`) e mock gateway contando chamadas:

1. `TestCompactionBeforeOverflow` (mandatório): mensagens crescem até a estimativa passar 70% de 2k; o mock registra a sequência de chamadas — a chamada de resumo (modelo `deepseek-v4.1-flash`, sem tools no request) ocorre **antes** da chamada que estouraria; a sessão continua funcionando (`EventTurnDone` no fim).
2. `TestCompactionPreservesLastFour` — pós-compação, `messages` = `[system] + últimas 4 originais`; as 4 caudas byte-idênticas.
3. `TestCompactionUsesFallbackModel` — `deepseek-v4.1-flash` ausente nos candidatos → resumo usa o default do catálogo.
4. `TestCompactionEmitsEventAndTranscript` — `EventCompaction` emitido com `TokensBefore > TokensAfter`; evento `compaction` no transcript com os dois campos.
5. `TestEstimateUsesRealUsageAfterFirstCall` — após primeira resposta do mock (usage conhecido), a estimativa reflete `lastPromptTokens + delta`; antes da primeira, usa heurística `/4`.
6. `TestTruncationRescuesGiantToolResult` — um único tool result de 50k chars numa janela de 2k: truncado para 2000 + sufixo; a chamada seguinte não estoura.
7. `TestTruncationPreservesTail` — tool result gigante dentro das últimas 4 mensagens não é truncado; compação o mantém intacto (cenário: janela grande o suficiente pós-resumo).
8. `TestSummaryFailureFallsBackToTruncation` — mock falha a chamada de resumo → truncamento executa e o turno completa sem `EventError`.

### Testes de Integração

Cobertos pelos testes de agent com mock gateway (fluxo completo decide→compact→call).

### Testes de E2E

Deferidos para `kspec-qa`: sessão longa real compacta e continua; linha `⚡ context compacted (Xk → Yk tokens)` visível em cor muted; coesão da tarefa pós-compação.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/agent`**: `lastPromptTokens` + `estimateTokens` + testes de estimativa — fundação observável.
2. **`internal/agent`**: `compact` (prompt de resumo, system message, evento) + `needsCompaction`.
3. **`internal/agent`**: `truncateOldToolResults` como garantia pós-compação.
4. **`internal/session`**: campos `TokensBefore/TokensAfter`.
5. **`internal/tui`**: caso `EventCompaction` + linha muted.
6. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib.
- Interação com techspec 004 (resume): o snapshot grava `messages` já compactadas — compatível por construção (snapshot serializa o estado corrente).

## Monitoramento e Observabilidade

### Error Tracking

Falha da chamada de resumo não é `EventError` — degrada para truncamento (silencioso por design). `EventError` continua reservado a falhas da chamada principal.

### Logging Estruturado

Transcript JSONL: evento `compaction` com `ts`, `tokens_before`, `tokens_after` — auditoria de quando/quantos turnos foram compactados. Sem dados sensíveis novos.

### Health Checks / Métricas / Alertas

Não aplicável — TUI local. O par antes/depois no transcript é a métrica de eficácia da compação.

## Considerações Técnicas

### Decisões Principais

1. **Role `system` para o resumo** (confirmado com o usuário): padrão OpenAI-compatible; introduz o conceito de system message no projeto (primeira uso — documentado aqui para as techspecs 004/010).
2. **Compação após `decide`**: a janela é do modelo escolhido — compactar antes do roteamento usaria a janela errada ao alternar entre deepseek (128k) e GLM (200k).
3. **Truncamento como garantia dura**: compação depende de LLM (pode falhar); truncamento é mecânico e sempre cabe — REQ-003 é o assoalho que torna "zero estouros" verdadeiro.
4. **Resumo não-roteado**: modelo fixo barato — rotear o resumo pelo Jev gastaria a chamada Typesafe para um trabalho mecânico.
5. **Heurística `/4` como fallback**: aproximação padrão para inglês/código; substituída pela medição real após a primeira resposta (REQ-001).

### Riscos Conhecidos

- **Estimativa imprecisa na primeira chamada** (sem medição): heurística `/4` pode subestimar tokens reais; mitigado pelo limiar conservador de 70% (folga de 30% da janela).
- **Resumo de baixa qualidade**: perda de detalhes antigos — inerente ao PRD; a cauda de 4 mensagens + resumo denso mitigam; a linha muted avisa o usuário do "esquecimento".
- **Custo da chamada de resumo**: um request extra por compação em modelo barato — aceitável versus falhar a sessão.
- **Loop de compação repetida**: sessão que cresce rápido pode compactar a cada step — comportamento correto (cada compação preserva a cauda); frequência monitorável via eventos `compaction` no transcript.

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing` + `httptest`, AAA, independência).
- Padrões do projeto: sem comentários no código, eventos via canal, receivers por valor na TUI, `strings.Builder` por ponteiro.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (sessão longa E2E), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/agent/agent.go` | `lastPromptTokens`, `estimateTokens`, `compact`, `truncateOldToolResults`, `EventCompaction` |
| `internal/agent/agent_test.go` | Catálogo de janela pequena, contagem de chamadas do mock, 8 cenários |
| `internal/catalog/catalog.go` | Sem mudança — `ContextWindow` e `DefaultModel` consumidos |
| `internal/catalog/models.yaml` | Sem mudança — janelas já declaradas |
| `internal/llm/llm.go` | Sem mudança — `ChatStream` sem tools reusado para o resumo |
| `internal/session/session.go` | Campos `TokensBefore/TokensAfter` no `Event` |
| `internal/tui/tui.go` | Caso `EventCompaction` — linha `⚡` muted |
| `internal/tui/tui_test.go` | Renderização da linha de compação |
