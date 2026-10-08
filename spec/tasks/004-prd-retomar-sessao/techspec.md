# Tech Spec — Retomar sessão (`kterminal --continue`)

## Requisitos Atendidos

- REQ-001 — Snapshot por turno
- REQ-002 — Carregamento de sessão
- REQ-003 — Flags CLI (`--continue`, `--session`)
- REQ-004 — Sessão retomada é a mesma sessão

## Resumo Executivo

O resume é construído sobre **snapshots**, não replay: ao final de cada turno (concluído, abortado ou em erro pós-stream), o agent grava no transcript um evento `{"type": "snapshot", "messages": [...]}` com o array `[]llm.Message` serializado inteiro — o snapshot é a fonte da verdade do estado da conversa. O carregamento lê o arquivo de trás para frente e usa o **último** snapshot; arquivo sem snapshot produz erro explícito. Duas flags novas: `--continue` (sessão mais recente por nome de arquivo) e `--session <caminho>`. A sessão retomada continua gravando no **mesmo arquivo** (o `Writer` ganha modo append sobre arquivo existente), o agent recebe as mensagens via `SetMessages` (que zera o cache de candidatos para revalidar contra o gateway), e a TUI reconstrói as últimas ~20 mensagens no viewport com os blocos existentes (user → userBox, assistant → markdown + `▣`, tool → linha de tool) + hint bar `resumed · N mensagens`.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/session/session.go`** (modificado — núcleo): `Event` ganha `Messages json.RawMessage`; `Writer.WriteSnapshot(messages []llm.Message)`; novo `AppendWriter(path) (*Writer, error)` (abre arquivo existente em `O_APPEND`); funções `Load(path) ([]llm.Message, error)` e `LoadLatest() (string, []llm.Message, error)`.
- **`internal/agent/agent.go`** (modificado): grava snapshot após `EventTurnDone`, `EventTurnAborted` e erro pós-stream; novo `SetMessages([]llm.Message)` (substitui `messages`, zera `candidates` e `sessionCost`).
- **`main.go`** (modificado): flags `--continue` e `--session <path>`; wiring do resume (load → `SetMessages` → `AppendWriter` no mesmo arquivo → sinaliza a TUI); erros de `--session` no stderr com exit 1.
- **`internal/tui/tui.go`** (modificado): reconstrução das últimas ~20 mensagens em blocos ao iniciar; hint bar `resumed · N mensagens`.

Fluxo de dados:

1. **Gravação (toda sessão)**: fim de turno → `WriteSnapshot(a.messages)` — evento JSONL com o array completo.
2. **Resume**: `main.go` → `LoadLatest()`/`Load(path)` → último snapshot → `agent.SetMessages(messages)` → `session.AppendWriter(pathOriginal)` → TUI recebe `resumed` com as mensagens para reconstrução visual.
3. **Continuação**: novos turnos appended no mesmo JSONL — histórico completo num lugar só.

## Design de Implementação

### Interfaces Principais

```go
func (w *Writer) WriteSnapshot(messages []llm.Message) error

func AppendWriter(path string) (*Writer, error)

func Load(path string) ([]llm.Message, error)

func LoadLatest() (path string, messages []llm.Message, err error)

func (a *Agent) SetMessages(messages []llm.Message)
```

Regras do carregamento:

- `Load`: lê todas as linhas do arquivo, itera de trás para frente, decodifica cada `Event`; primeiro `Type == "snapshot"` vence — `json.Unmarshal` do campo `Messages` em `[]llm.Message`. Fim do arquivo sem snapshot → `errors.New("sessão sem snapshot")`.
- `LoadLatest`: `os.ReadDir(session.Dir())`, filtra `*.jsonl`, ordena por nome (timestamp embutido no nome), pega o último, chama `Load`. Sem arquivos → erro específico `ErrNoSessions` (o caller distingue "sem sessões" de "sessão corrompida").
- `WriteSnapshot`: `json.Marshal(messages)` → `Event{Type: "snapshot", Messages: raw}` → `Write` normal (ts preenchido).
- `SetMessages`: `a.messages = messages; a.candidates = nil; a.sessionCost = 0` — `candidates = nil` força `ensureCandidates` a revalidar contra o gateway no próximo turno (modelos/candidatos podem ter mudado entre sessões — restrição do PRD).
- `main.go`:

```go
if *continueFlag {
	path, messages, err := session.LoadLatest()
	if err != nil {
		// sem sessões: aviso no chat, app segue fresh
	} else {
		agent.SetMessages(messages)
		sess = session.AppendWriter(path)
		resumed = len(messages)
	}
}
if *sessionFlag != "" {
	messages, err := session.Load(*sessionFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kterminal:", err)
		os.Exit(1)
	}
	agent.SetMessages(messages)
	sess = session.AppendWriter(*sessionFlag)
	resumed = len(messages)
}
```

- `--continue` sem sessões: a TUI recebe um bloco inicial `colTextMuted` "no previous session — starting fresh" e o app segue normalmente (REQ-003).
- `--session` com arquivo inexistente/sem snapshot: erro claro no stderr, exit 1 (REQ-003).
- `--continue` e `--session` juntos: `--session` vence (mais específico); documentado no help.

### Modelos de Dados

```go
type Event struct {
	// campos existentes...
	Messages json.RawMessage `json:"messages,omitempty"`
}
```

- Snapshot serializa `[]llm.Message` incluindo `ToolCalls`/`ToolCallID` — o par tool_call↔tool result exigido pelo gateway sobrevive ao resume intacto (motivo central de snapshot vs replay).
- `session` importa `kterminal/internal/llm` apenas para o tipo `Message` em `WriteSnapshot`/`Load` — sem ciclo (`llm` não importa `session`).

### Reconstrução na TUI

- Ao iniciar com sessão carregada, a TUI recebe as mensagens (via campo/param no construtor, ex.: `tui.New(ag, cfg, cat, dark, tui.WithResumed(messages))`).
- Reconstrução: **últimas ~20 mensagens** (confirmado com o usuário) — role `user` → bloco userBox (texto), role `assistant` → markdown + linha `▣`, role `tool` → linha de tool (`  resultado` truncado como hoje). Mensagens `system` (pós-compação, techspec 003) não renderizam bloco.
- O agente recebe o histórico **completo** via snapshot — só a renderização é limitada (viewport enxuto; nada se perde na conversa).
- Hint bar: `resumed · N mensagens` (N = total do snapshot) enquanto a sessão durar.

## Pontos de Integração

- **Gateway LLM**: `SetMessages` zera `candidates` → `ensureCandidates` revalida `/v1/models` no primeiro turno pós-resume; mensagens carregadas com modelos que saíram do gateway degradam normalmente (o Jev só recebe a interseção atual).
- **Pinned model** (`/model`): não persiste entre sessões (estado em memória) — comportamento atual preservado; fora de escopo.

## Verificações Técnicas

### Segurança

- Arquivo de sessão é 0600 (padrão existente); `AppendWriter` preserva o modo do arquivo existente.
- `Load` rejeita JSON malformado com erro explícito — nunca crash; snapshot parcial (linha truncada no fim do arquivo por crash anterior) é ignorado naturalmente (linha inválida no meio do scan de trás para frente pula para a anterior; sem snapshot válido → erro "sessão sem snapshot").
- Validação de tamanho: arquivo de sessão gigante é lido uma vez — sem streaming; aceitável para JSONL de sessões locais.

### Arquitetura

- Snapshot é a fonte da verdade (requisito não negociável do PRD): nenhuma reconstrução a partir de eventos granulares; `Load` olha **só** eventos `snapshot`.
- Direção de dependência: `main → session/agent/tui`; `session → llm` (tipo Message); `tui` não toca `session` (recebe mensagens prontas).
- Snapshot por turno (não por evento): custo de escrita O(mensagens) por turno — amortizado e simples; JSONL cresce, mas é o formato existente (fora de escopo compactar).

### Infraestrutura

- Sem novos requisitos — stdlib (`os`, `encoding/json`, `sort`). Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/session/session_test.go`**:

1. `TestSnapshotRoundtrip` (mandatório): Writer em temp dir → 3 turnos simulados com `WriteSnapshot` → `Load(w.Path())` retorna mensagens byte-idênticas às originais (comparar JSON marshalado), incluindo mensagens com `ToolCalls`/`ToolCallID`.
2. `TestLoadUsesLastSnapshot`: dois snapshots (estado A, estado B) → `Load` retorna B.
3. `TestLoadWithoutSnapshot`: arquivo com eventos `user`/`assistant` mas sem snapshot → erro "sessão sem snapshot".
4. `TestLoadLatestPicksNewest`: três arquivos com timestamps crescentes → `LoadLatest` retorna o caminho e mensagens do mais recente; diretório vazio → `ErrNoSessions`.
5. `TestAppendWriterAppends`: `AppendWriter` sobre arquivo existente → novos eventos appended, conteúdo anterior intacto, `Path()` = caminho original.

**`internal/agent/agent_test.go`**:

6. `TestSnapshotWrittenAfterTurnDone`: turno completo via mock → transcript contém evento `snapshot` com as mensagens do turno.
7. `TestSnapshotWrittenAfterAbort`: turno abortado (cancel) → snapshot gravado com o estado consistente (tool results sintéticos incluídos).
8. `TestSetMessagesClearsCandidates`: `SetMessages` → `ensureCandidates` refaz `ListModels` no próximo `Run` (mock conta chamadas).

**`internal/tui/tui_test.go`**:

9. `TestResumedReconstructsBlocks`: modelo com 25 mensagens carregadas → viewport contém os blocos das últimas ~20 (user box, markdown, linha de tool), não contém a mensagem mais antiga.
10. `TestHintBarShowsResumed`: render contém `resumed · N mensagens`.

### Testes de Integração

`main.go` wiring é fino (validado em E2E); os testes de session/agent cobrem o contrato ponta a ponta.

### Testes de E2E

Deferidos para `kspec-qa`: conversar → sair → `kterminal --continue` → pergunta nova responde com contexto anterior; `--session` inexistente → stderr + exit 1; `--continue` sem sessões → aviso no chat e app funcional; histórico visível no viewport ao iniciar.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/session`**: campo `Messages`, `WriteSnapshot`, `Load`, `LoadLatest`, `AppendWriter` + testes de roundtrip — fundação isolada e testável.
2. **`internal/agent`**: gravação de snapshot nos três pontos de fim de turno + `SetMessages`.
3. **`main.go`**: flags `--continue`/`--session`, wiring, erros/exit codes.
4. **`internal/tui`**: reconstrução de blocos + hint bar `resumed`.
5. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib.
- Compatibilidade com techspec 003 (compação): snapshots gravam `messages` pós-compação (system message incluída) — `Load` devolve o estado compactado fiel; `llm.Message` com content parts (techspec 008) serializa transparentemente via `json.RawMessage`.

## Monitoramento e Observabilidade

### Error Tracking

Falhas de resume são explícitas e acionáveis: `--session` → stderr + exit 1; `--continue` sem sessões → aviso no chat. `Load` com JSON corrompido → erro explícito, nunca silencioso (REQ do PRD "nunca crash").

### Logging Estruturado

Transcript JSONL: evento `snapshot` com `ts` + `messages` — auditoria do estado exato da conversa a cada turno. Sem dados sensíveis além do conteúdo já gravado.

### Health Checks / Métricas / Alertas

Não aplicável — TUI local. O hint bar `resumed · N mensagens` é o indicador de estado visível ao usuário.

## Considerações Técnicas

### Decisões Principais

1. **Snapshot em vez de replay** (requisito não negociável do PRD): eventos granulares não carregam o par tool_call↔tool result com fidelidade de contrato OpenAI; o array serializado é o estado exato que o gateway viu.
2. **Últimas ~20 mensagens no viewport** (confirmado com o usuário): renderização limitada para iniciar rápido e viewport enxuto; o agente recebe o histórico completo — nada funcional é perdido.
3. **`AppendWriter` no mesmo arquivo** (REQ-004): histórico completo num lugar só; alternativa (arquivo novo por resume) rejeitada — fragmentaria a auditoria.
4. **`SetMessages` zera `candidates`**: revalidação contra o gateway é exigida pelo PRD (modelos mudam entre sessões); custo de um `ListModels` extra no primeiro turno.
5. **Snapshot também em abort/erro pós-stream**: o estado pós-abort (com tool results sintéticos) é válido e retomável — perder esse snapshot deixaria sessões abortadas não-retomáveis.

### Riscos Conhecidos

- **Crescimento do JSONL**: snapshot por turno duplica informação do turno — aceito (formato append-only existente; compactação de transcript fora de escopo).
- **Snapshot com mensagens de modelo antigo**: mensagens históricas referenciam modelos que saíram do catálogo — inofensivo (o gateway recebe o histórico como está; o roteamento é por chamada nova).
- **Crash no meio do `Write`**: linha parcial no fim do arquivo — `Load` ignora linha inválida e usa o snapshot anterior; degradação graciosa.
- **`--continue` pega sessão de outro projeto**: `sessions/` é global por usuário — a sessão mais recente pode ser de diretório diferente; aceito na v1 (picker interativo fora de escopo).

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing` + `httptest`, AAA, independência).
- Padrões do projeto: sem comentários no código, eventos via canal, receivers por valor na TUI, `strings.Builder` por ponteiro.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (fluxo conversar→sair→retomar), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/session/session.go` | `Messages` no `Event`, `WriteSnapshot`, `Load`, `LoadLatest`, `AppendWriter` |
| `internal/session/session_test.go` | Roundtrip, último snapshot, sem snapshot, `LoadLatest`, append |
| `internal/agent/agent.go` | Snapshot nos fins de turno, `SetMessages` |
| `internal/agent/agent_test.go` | Snapshot pós-turno/abort, `SetMessages` revalida candidatos |
| `main.go` | Flags `--continue`/`--session`, wiring, exit codes |
| `internal/tui/tui.go` | Reconstrução de blocos, hint bar `resumed` |
| `internal/tui/tui_test.go` | Reconstrução das últimas ~20, hint bar |
| `internal/llm/llm.go` | Sem mudança — `Message` serializada como está |
