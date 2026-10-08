# Tech Spec — Diff colorido para write/edit

## Requisitos Atendidos

- REQ-001 — Diff pós-execução
- REQ-002 — Diff antes da aprovação (modo `--confirm`)
- REQ-003 — Diff no transcript

## Resumo Executivo

O diff de linhas é implementado à mão em `internal/tools/diff.go` (LCS clássico, sem dependências). As tools `write` e `edit` passam a capturar o conteúdo anterior do arquivo e calcular o diff unificado; `Registry.Execute` muda o retorno de `string` para um struct `Result{Output, Diff}`, propagando o diff pelos eventos do agente (`EventToolResult` e `EventConfirm`) até a TUI e o transcript. Para o modo `--confirm`, onde o diff precisa existir **antes** da aprovação, um novo método `Registry.PendingDiff` simula a edição em memória (lê o conteúdo atual, aplica a transformação sem tocar o disco) — o arquivo só é mutado após o `y`. A TUI ganha duas cores novas na paleta (`colDiffAdded #4fd6be`, `colDiffRemoved #c53b53`, do tema original do opencode) e renderiza o diff como bloco colorido com truncamento no meio (~40 linhas visíveis). O transcript grava as linhas formatadas no evento `tool_result`.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/tools/diff.go`** (novo): `DiffLine{Kind, Text}` e `LineDiff(old, new string) []DiffLine` — algoritmo LCS com DP matrix; arquivos de código são pequenos o suficiente para O(n·m).
- **`internal/tools/fs.go`** (modificado): `write` lê o conteúdo anterior antes de sobrescrever (arquivo novo → todas as linhas como `+`); `edit` captura o `data` que já lê hoje e calcula o diff antes↔depois. Edições sem mudança produzem diff vazio.
- **`internal/tools/tools.go`** (modificado): `Registry.Execute` retorna `Result{Output string, Diff []DiffLine}`; novo `Registry.PendingDiff(ctx, name, argsJSON) ([]DiffLine, error)` — simula `write`/`edit` em memória, nunca escreve.
- **`internal/agent/agent.go`** (modificado): `Event` ganha campo `Diff []tools.DiffLine`; no fluxo de confirmação, chama `PendingDiff` antes de emitir `EventConfirm`; após execução, propaga `Result.Diff` no `EventToolResult` e no `session.Event`.
- **`internal/tui/theme.go`** (modificado): `colDiffAdded #4fd6be`, `colDiffRemoved #c53b53`.
- **`internal/tui/tui.go`** (modificado): renderização do bloco de diff após a linha de resultado e dentro de `confirmView()`; truncamento no meio com `… N more lines …`.
- **`internal/session/session.go`** (modificado): `Event` ganha campo `Diff []string` (linhas já formatadas com prefixo `+`/`-`/espaço).

Fluxo de dados:

1. **Pós-execução (sempre)**: tool executa → `Result.Diff` → `EventToolResult{Diff}` → TUI renderiza bloco colorido + transcript grava `Diff []string`.
2. **Pré-aprovação (`--confirm`)**: agente identifica tool mutante `write`/`edit` → `PendingDiff` (simulação em memória) → `EventConfirm{Diff}` → `confirmView()` exibe o diff completo antes do y/n.
3. **Diff vazio** (edição que não muda nada): sem bloco na TUI, campo omitido no JSONL (`omitempty`).

## Design de Implementação

### Interfaces Principais

```go
type DiffLine struct {
	Kind byte
	Text string
}

func LineDiff(old, new string) []DiffLine

type Result struct {
	Output string
	Diff   []DiffLine
}

func (r *Registry) Execute(ctx context.Context, name, argsJSON string) (Result, error)

func (r *Registry) PendingDiff(ctx context.Context, name, argsJSON string) ([]DiffLine, error)
```

`PendingDiff` por tool:

- `edit`: lê o arquivo, valida `old_string` único (mesmas regras da tool), aplica `strings.Replace` **em memória**, retorna `LineDiff(data, simulated)`.
- `write`: lê o arquivo se existir (senão `""`), retorna `LineDiff(existing, content)`.
- Outras tools: retorna `nil, nil`.

Regra do agente no fluxo de confirmação:

```go
if a.Confirm && a.Tools.IsMutating(name) {
	diff, _ := a.Tools.PendingDiff(ctx, name, args)
	ch := make(chan bool, 1)
	a.emit(Event{Kind: EventConfirm, Tool: name, Args: args, Diff: diff, ApproveCh: ch})
	if !<-ch {
		toolResult = "user declined this tool call"
	} else {
		res, err = a.Tools.Execute(ctx, name, args)
	}
}
```

Erro de `PendingDiff` é engolido (diff `nil`) — a confirmação nunca deixa de aparecer por falha na simulação.

### Modelos de Dados

```go
type Event struct {
	// campos existentes...
	Diff []tools.DiffLine
}
```

- `session.Event` ganha `Diff []string` com tag `json:"diff,omitempty"` — linhas formatadas (`+foo`, `-bar`, ` baz`), prontas para auditoria sem reler arquivos.
- `agent.Event.Diff` usa `[]tools.DiffLine` (tipado); a formatação para string acontece na gravação do transcript e na renderização.

### Renderização na TUI

- Bloco de diff renderizado imediatamente após a linha de resultado da tool (`EventToolResult`) ou antes do y/n (`confirmView`).
- Cada linha: prefixo `+` em `colDiffAdded`, `-` em `colDiffRemoved`, espaço (contexto) em `colTextMuted`.
- Truncamento: > 40 linhas → manter primeiras ~20 e últimas ~20, inserir `… N more lines …` em `colTextMuted` no meio.
- `confirmView()` rola o diff quando ele excede a altura disponível do overlay (o diff completo permanece acessível; a versão v1 usa o mesmo truncamento de 40 linhas do chat).

## Pontos de Integração

- **Nenhuma integração externa nova** — diff local, sem rede.
- Contrato com o gateway não muda: o `Output` da tool continua sendo a única coisa que volta ao LLM como `role: "tool"`; o diff é apresentação/auditoria, invisível ao modelo.

## Verificações Técnicas

### Segurança

- `PendingDiff` nunca escreve no disco — a decisão de aprovação acontece com o arquivo intocado (requisito do fluxo `--confirm`).
- Conteúdo de arquivos no diff/transcript: mesma sensibilidade do transcript existente (conteúdo da conversa); sem dados novos além do que as tools já leem.

### Arquitetura

- Direção de dependência inalterada: `tui → agent → tools`; `DiffLine` vive em `tools` e é referenciado por `agent`/`session`/`tui` como tipo — `session` não pode importar `tools` (import cycle não existe: `session` recebe `[]string` formatado do agent, não o tipo).
- Mudança de assinatura de `Execute` é mecânica: único caller é o agent.
- LCS com DP matrix: memória O(n·m) bytes — arquivo de 100k linhas é irreal no fluxo de edição; guard: se qualquer lado exceder 10k linhas, o diff degrada para "todas as linhas removidas + adicionadas" (sem DP), evitando alocação gigante.

### Infraestrutura

- Sem novos requisitos — stdlib puro. Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/tools/diff_test.go`**:

1. `TestLineDiffInsertion` — linha nova no fim → uma `+`.
2. `TestLineDiffRemoval` — linha removida → uma `-`.
3. `TestLineDiffMiddleChange` — mudança no meio → `-` e `+` adjacentes com contexto preservado.
4. `TestLineDiffEqual` — conteúdos iguais → diff vazio.
5. `TestLineDiffEmpty` — arquivo vazio → novo → tudo `+`.
6. `TestLineDiffLargeFallback` — > 10k linhas → degrada sem DP.

**`internal/tools`** (fs/tools):

7. `TestWriteToolDiffOnExistingFile` — write em arquivo existente retorna diff com `-` e `+`.
8. `TestWriteToolDiffNewFile` — arquivo novo → todas `+`.
9. `TestEditToolDiff` — edit retorna diff antigo↔novo.
10. `TestPendingDiffDoesNotTouchDisk` — `PendingDiff` de edit/write: conteúdo do arquivo em disco idêntico antes/depois; diff correto.
11. `TestPendingEditValidation` — `old_string` inexistente/duplicado retorna erro igual à tool.

**`internal/tui`**:

12. `TestDiffBlockRendersColors` — `EventToolResult` com diff → bloco contém ANSI de `colDiffAdded` (`\x1b[38;2;79;214;190`) e `colDiffRemoved`.
13. `TestDiffBlockTruncatesMiddle` — diff de 100 linhas → ~40 visíveis + `… N more lines …`.
14. `TestEmptyDiffRendersNoBlock` — diff vazio → nenhum bloco extra.
15. `TestConfirmViewShowsDiff` — `pendingConfirm` com diff → view contém linhas `+`/`-` antes do hint y/n.

**`internal/agent`**:

16. `TestToolResultEventCarriesDiff` — mock gateway responde com tool call `write` → `EventToolResult` carrega o diff; transcript gravado contém campo `diff`.

### Testes de Integração

Cobertos pelos testes de agent com mock gateway (16) — o fluxo ponta a ponta tool→evento→transcript.

### Testes de E2E

Deferidos para `kspec-qa`: edit real visível como diff colorido no chat; modo `--confirm` exibindo diff antes do y/n; arquivo grande truncado de forma legível.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/tools/diff.go`** + testes — algoritmo isolado, sem dependência do resto.
2. **`internal/tools/fs.go`** — captura do conteúdo anterior e cálculo do diff em `write`/`edit`.
3. **`internal/tools/tools.go`** — `Result{Output, Diff}`, `PendingDiff`; atualizar `bash`/`read`/`glob`/`grep` mecanicamente (diff `nil`).
4. **`internal/agent`** — campo `Diff` no `Event`, `PendingDiff` no fluxo de confirmação, propagação no `EventToolResult` e no transcript.
5. **`internal/session`** — campo `Diff []string`.
6. **`internal/tui`** — cores novas no tema, bloco de diff, `confirmView` com diff.
7. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib (`strings`, `fmt`).
- Conflito conhecido de sequenciamento: a techspec 009 (diagnósticos) também modifica `Registry.Execute`/`fs.go` — implementar 002 primeiro; 009 anexa diagnóstico ao `Output` do `Result` sem tocar no `Diff`.

## Monitoramento e Observabilidade

### Error Tracking

Falhas de `PendingDiff` são engolidas (diff ausente, confirmação funcional) — não há novo tipo de erro. Erros das tools seguem o fluxo existente (`EventError`).

### Logging Estruturado

Transcript JSONL: evento `tool_result` ganha `diff []string` (linhas formatadas) — rastreabilidade REQ-003 sem reler arquivos. Sem dados sensíveis além do conteúdo já gravado hoje.

### Health Checks / Métricas / Alertas

Não aplicável — TUI local, sem servidor.

## Considerações Técnicas

### Decisões Principais

1. **`Result` struct em vez de segundo retorno** (confirmado): `Execute(ctx, name, args) (Result, error)` — extensível (009 adiciona diagnóstico no `Output` sem nova assinatura) e nomeado na documentação.
2. **Simulação em memória para o diff pré-aprovação** (confirmado com o usuário): `PendingDiff` aplica a transformação sem escrever — o disco nunca é mutado antes do `y`. Alternativa rejeitada (aplicar + reverter com backup): muta o arquivo antes da aprovação e adiciona superfície de falha (crash entre aplicar e reverter).
3. **LCS clássico à mão** (PRD): sem dependências; arquivos de código são pequenos. Guard de 10k linhas para degradar graciosamente.
4. **Diff como apresentação, não como conteúdo ao modelo**: o LLM continua vendo só `Output` — mudar o que o modelo vê está fora do PRD e alteraria custos/comportamento.
5. **`Diff []string` no transcript** (linhas formatadas): reconstrução direta na auditoria; `omitempty` mantém eventos de tools não-editoras idênticos aos atuais.

### Riscos Conhecidos

- **Race diff↔conteúdo**: entre `PendingDiff` (confirmação) e `Execute` (pós-aprovação) o arquivo pode mudar (ex.: outro processo) — o diff aplicado pode divergir do exibido; janela minúscula, aceito na v1 (o diff pós-execução, sempre calculado no momento da escrita, é a fonte da verdade).
- **Diff de arquivo binário**: fora de escopo (PRD); `LineDiff` em conteúdo binário produz linhas ilegíveis mas não quebra — aceitável.
- **Custo do LCS**: guard de 10k linhas mitiga; edição normal de código é ordens de magnitude menor.

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/` consistentes; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/Java/Angular/React/tests.md (Vitest)/logging.md (console): não aplicáveis — stack Go; padrões do repositório prevalecem (`testing` + `httptest`, AAA, independência).
- Padrões do projeto: sem comentários no código, eventos via canal (buffer 512, emit não-bloqueante), receivers por valor na TUI, `strings.Builder` por ponteiro.
- Skills kspec aplicáveis: `kspec-tasks` (decomposição), `kspec-implement`, `kspec-qa` (diff visível no chat/confirm), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/tools/diff.go` | Novo — `DiffLine`, `LineDiff` (LCS) |
| `internal/tools/diff_test.go` | Novo — testes do algoritmo |
| `internal/tools/fs.go` | Captura do conteúdo anterior; diff em `write`/`edit` |
| `internal/tools/tools.go` | `Result{Output, Diff}`, `PendingDiff` |
| `internal/agent/agent.go` | Campo `Diff` no `Event`; `PendingDiff` no confirm; propagação |
| `internal/agent/agent_test.go` | Teste de propagação evento/transcript |
| `internal/session/session.go` | Campo `Diff []string` no `Event` |
| `internal/tui/theme.go` | `colDiffAdded #4fd6be`, `colDiffRemoved #c53b53` |
| `internal/tui/tui.go` | Bloco de diff, truncamento, `confirmView` com diff |
| `internal/tui/tui_test.go` | Testes de renderização (cores, truncamento, confirm) |
