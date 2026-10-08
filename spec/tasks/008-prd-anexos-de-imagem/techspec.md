# Tech Spec — Anexos de imagem no prompt

## Requisitos Atendidos

- REQ-001 — Mensagens com content parts
- REQ-002 — Catálogo com capacidade de visão
- REQ-003 — Comandos de anexo (`/image`, `/unimage`, chips)
- REQ-004 — Roteamento por visão
- REQ-005 — Transcript sem base64

## Resumo Executivo

`llm.Message` ganha suporte a content parts OpenAI-compatible: campo `ContentParts []ContentPart` com `MarshalJSON` customizado — sem parts o request é byte a byte igual ao atual (`"content": "string"`), com parts o content vira array (`text` + `image_url` com data URI base64). O catálogo ganha `vision: true|false` no YAML (`glm-5.3`: true; demais: false). A TUI gerencia anexos pendentes: `/image <caminho>` valida (extensão png/jpg/jpeg/gif/webp, 5MB, existência), lê e codifica; chips `🖼 nome ×` em `colSecondary` acima do prompt; `/unimage <n>` remove. No envio com anexos, o agent recebe a mensagem com parts; o `decide()` filtra candidatos para `Vision == true` antes de consultar o Jev (state menciona os anexos); sem modelos com visão → erro amigável e anexos preservados. O base64 vai **só** ao gateway — o transcript grava metadados (nome, tamanho), nunca o conteúdo (requisito não negociável de privacidade).

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/llm/llm.go`** (modificado): `ContentPart{Type, Text, ImageURL}`, `ImageURL{URL}`; `Message.ContentParts` + `MarshalJSON` custom (string quando vazio, array quando populado).
- **`internal/catalog/`** (modificado): `models.yaml` ganha `vision` por modelo; `Model.Vision bool`.
- **`internal/tui/tui.go`** (modificado): `pendingAttachments []attachment{name, size, dataURI}`; comandos `/image`/`/unimage`; chips acima do prompt; envio com anexos via novo método do agent.
- **`internal/agent/agent.go`** (modificado): `RunWithAttachments(text string, parts []llm.ContentPart)`; filtragem de candidatos por `Vision` quando a mensagem tem imagens; state do Jev menciona anexos; erro amigável sem candidatos de visão.
- **`internal/session/session.go`** (modificado): `Event` ganha `Attachments []AttachmentMeta` (nome, tamanho) — gravado no evento `user`.

Fluxo de dados:

1. **Anexo**: `/image screenshot.png` → validação + leitura + base64 data URI → chip visível.
2. **Envio**: Enter com anexos → `RunWithAttachments` → mensagem user com `ContentParts` → filtragem de candidatos `Vision` → Jev decide entre válidos → `ChatStream` serializa o array → gateway recebe `image_url`.
3. **Privacidade**: transcript grava `attachments: [{name, size}]` no evento `user`; o base64 existe só no request HTTP.

## Design de Implementação

### Interfaces Principais

```go
type ImageURL struct {
	URL string `json:"url,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error)
```

`MarshalJSON`:

- `len(m.ContentParts) == 0` → `{"role": ..., "content": "<string>", ...}` — idêntico ao atual (golden test garante compatibilidade byte a byte).
- Com parts → `"content": [{"type":"text","text":...}, {"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}]`.

Agent:

```go
func (a *Agent) RunWithAttachments(text string, parts []llm.ContentPart, meta []session.AttachmentMeta)

func (a *Agent) Run(userInput string)
```

- `Run` vira wrapper de `RunWithAttachments` com parts nil — callers existentes inalterados.
- Filtragem em `decide` quando `hasImages`: `candidates = filter(candidates, m.Vision)`; vazio → `emitError("no vision-capable model available")` **sem consumir a mensagem** (a user message não é anexada ao histórico; anexos permanecem pendentes na TUI para o usuário remover — REQ-004).
- State do Jev com anexos: sufixo `"This step includes image attachments."` no `buildState`.

TUI:

- `attachment{name string, size int64, dataURI string}`; `/image <path>`: valida extensão (map `png/jpg/jpeg/gif/webp` → mime), `os.Stat` ≤ 5MB, lê, `base64.StdEncoding` → `data:<mime>;base64,<...>`; erros inline em `colError` sem abortar o prompt.
- `/image` sem args → lista anexos pendentes no chat; `/unimage <n>` → remove pelo índice (1-based).
- Chips: linha acima do prompt box, `🖼 nome ×` em `colSecondary`; enter envia texto + anexos e limpa pendentes (exceto no erro de roteamento, que os preserva).
- Bloco do usuário no chat: texto + chips dos anexos enviados.

### Modelos de Dados

```yaml
- name: glm-5.3
  vision: true
```

```go
type Model struct {
	// campos existentes...
	Vision bool `yaml:"vision"`
}
```

```go
type AttachmentMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}
```

- `session.Event` ganha `Attachments []AttachmentMeta` (`json:"attachments,omitempty"`) — REQ-005: metadados apenas.
- Compatibilidade com techspec 004 (snapshot): `Message` com parts serializa no snapshot via `MarshalJSON` — retomar sessão com imagem recupera as parts fielmente (o base64 volta ao gateway, nunca ao JSONL de eventos).

## Pontos de Integração

- **Gateway LLM (LiteLLM)**: formato OpenAI-compatible de content parts — nenhum endpoint novo; o gateway roteia ao modelo com visão. Modo de falha: modelo declarado `vision` no catálogo mas rejeitando imagem no gateway → erro de API segue o fluxo `EventError` existente (mensagem clara do gateway).
- **Jev (Typesafe)**: contrato inalterado — recebe menos candidatos (filtrados) e state com menção a anexos; a escolha continua dele.

## Verificações Técnicas

### Segurança

- **Base64 só ao gateway** (requisito não negociável): o transcript grava metadados; o state do Jev menciona a existência dos anexos, nunca o conteúdo; chips mostram nome/tamanho.
- Limite de 5MB por imagem: protege payload do request; validação antes da leitura (`os.Stat`).
- Extensões permitidas (png/jpg/jpeg/gif/webp): whitelist, não blacklist.
- Data URI com mime correto por extensão — sem execução de conteúdo, apenas transporte.

### Arquitetura

- `MarshalJSON` custom é o ponto único de compatibilidade: golden test trava o formato string (sem anexos) — regressão de contrato é impossível de passar despercebida.
- Filtragem no agent (não nos routers): routers permanecem genéricos (não conhecem visão); o agent é o dono do contexto da chamada.
- Erro de roteamento sem consumo da mensagem: o turno não começa (`messages` intocado) — o usuário remove anexos e reenvia; sem estado corrompido.

### Infraestrutura

- Sem novos requisitos — stdlib (`encoding/base64`, `os`). Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/llm/llm_test.go`**:

1. `TestMessageMarshalStringContentGolden` — sem parts → JSON byte a byte igual ao golden atual (compatibilidade REQ-001).
2. `TestMessageMarshalContentParts` — com parts → array com `text` + `image_url` (data URI) na ordem.
3. `TestMessageUnmarshalRoundtrip` — marshal → unmarshal → campos preservados (snapshot da techspec 004).

**`internal/catalog`**:

4. `TestVisionParsedFromYAML` — `glm-5.3` → `Vision: true`; demais → false.

**`internal/tui/tui_test.go`** (temp dir):

5. `TestImageCommandAttaches` — `/image shot.png` (arquivo válido) → chip visível, anexo pendente com data URI.
6. `TestImageRejectsInvalid` — inexistente / `.txt` / 6MB → erro inline, nada anexado.
7. `TestUnimageRemovesAttachment` — 2 anexos + `/unimage 1` → segundo permanece.
8. `TestSendWithAttachmentsBuildsParts` — Enter com anexos → `RunWithAttachments` recebe parts com `image_url`; bloco do chat mostra texto + chips; pendentes limpos.
9. `TestImageWithoutArgsLists` — `/image` → lista no chat.

**`internal/agent/agent_test.go`** (mock gateway + mock Jev):

10. `TestVisionFilterRoutesOnlyVisionModels` — mensagem com imagem → mock Jev recebe só candidatos `Vision == true`; state contém "image attachments".
11. `TestNoVisionModelPreservesAttachments` — gateway sem modelos de visão → `EventError` "no vision-capable model available"; `messages` não ganhou a user message (turno não consumido).
12. `TestRequestContainsContentParts` — mock gateway decodifica o request: `messages[0].content` é array com `image_url` (data URI).
13. `TestTranscriptHasNoBase64` — evento `user` no JSONL contém `attachments: [{name, size}]` e nenhum `base64,` no arquivo.

### Testes de Integração

Cobertos pelos testes de agent (10-13) — TUI→agent→gateway→transcript com mocks.

### Testes de E2E

Deferidos para `kspec-qa`: `/image screenshot.png` + pergunta → resposta descreve a imagem; sem anexos → request idêntico ao anterior (compatibilidade); erro amigável sem modelo de visão com anexos preservados.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/llm`**: `ContentPart`/`ImageURL` + `MarshalJSON` + golden tests — fundação de contrato.
2. **`internal/catalog`**: campo `vision` no YAML + parse + testes.
3. **`internal/tui`**: comandos `/image`/`/unimage`, validação, chips.
4. **`internal/agent`**: `RunWithAttachments`, filtragem por visão, state, erro amigável.
5. **`internal/session`**: `AttachmentMeta` no evento `user`.
6. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib.
- Interação com techspec 004: snapshot serializa `Message` com parts (marshal custom) — resume de sessão com imagem funciona; `Load` devolve parts intactas.
- Interação com techspec 003: compação preserva a mensagem com parts na cauda (se entre as últimas 4) ou a envia ao resumo como texto (parts de imagem viram `[image attached: nome]` no prompt de resumo — decisão documentada aqui).

## Monitoramento e Observabilidade

### Error Tracking

Erros de anexo (arquivo grande, formato inválido) são inline na TUI — sem `EventError`. Falha de roteamento por visão é `EventError` amigável com anexos preservados.

### Logging Estruturado

Transcript JSONL: evento `user` com `attachments [{name, size}]` — auditoria do que foi anexado sem vazar conteúdo (REQ-005). Verificação de não-vazamento é teste automático (13).

### Health Checks / Métricas / Alertas

Não aplicável — TUI local.

## Considerações Técnicas

### Decisões Principais

1. **`MarshalJSON` condicional em vez de `Content any`**: manter `Content string` + parts opcionais preserva o tipo forte e o request atual byte a byte — `any` quebraria todos os callers e a compatibilidade (REQ-001 é requisito duro).
2. **Filtragem no agent, routers intocados**: visão é restrição de candidato, não política de roteamento — o Jev continua decidindo entre os válidos (REQ-004: "mantendo o Jev como decidor").
3. **Turno não consumido no erro de visão**: preservar anexos exige não anexar a user message — o reenvio pós-remover não duplica mensagens.
4. **Data URI base64 inline** (formato OpenAI): sem upload para serviço externo — a imagem vai direto ao gateway do usuário; sem superfície de armazenamento nova.
5. **`vision: false` como default honesto**: só `glm-5.3` confirmado (PRD); errar para false degrada para texto (seguro); errar para true quebraria chamadas.

### Riscos Conhecidos

- **Modelo `vision: true` indisponível no gateway**: filtragem usa a interseção catálogo×gateway (`candidates`) — indisponível já cai fora; resta o caso "nenhum disponível" com erro amigável (coberto).
- **Custo de tokens de imagem**: imagens grandes custam tokens no gateway — limite de 5MB mitiga; estimativa de tokens da techspec 003 não conta pixels (imprecisão aceita; a medição real da resposta corrige o `lastPromptTokens` no turno seguinte).
- **Snapshot com base64 no JSONL**: o snapshot (techspec 004) serializa `messages` — se uma mensagem com imagem estiver no histórico, o snapshot conteria o base64, violando o espírito do REQ-005. Mitigação: `WriteSnapshot` serializa parts de imagem como placeholder `{"type":"image_url","image_url":{"url":"[omitted]"}}` — o resume de sessão com imagem perde o anexo (aceitável: imagens são efêmeras; documentado como limitação).
- **Gateway rejeita content parts em modelo sem visão**: impossível por construção (filtragem), exceto pin manual (`/model` num modelo sem visão com anexo ativo) — o pin vence a filtragem? Decisão: com anexos, o pin em modelo sem visão é ignorado e segue-se a filtragem normal (o request quebraria de outro jeito).

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing` + `httptest`, golden files, AAA).
- Padrões do projeto: sem comentários no código, eventos via canal, receivers por valor na TUI, paleta existente (chips em `colSecondary`).
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (fluxo E2E de anexo + roteamento), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/llm/llm.go` | `ContentPart`, `ImageURL`, `MarshalJSON` condicional |
| `internal/llm/llm_test.go` | Golden string content, parts array, roundtrip |
| `internal/catalog/models.yaml` | `vision` por modelo (glm-5.3: true) |
| `internal/catalog/catalog.go` | `Model.Vision` |
| `internal/tui/tui.go` | `/image`, `/unimage`, chips, envio com anexos |
| `internal/tui/tui_test.go` | Comandos, validações, chips, parts construídas |
| `internal/agent/agent.go` | `RunWithAttachments`, filtragem por visão, state, erro amigável |
| `internal/agent/agent_test.go` | Filtro, preservação de anexos, request com parts, transcript sem base64 |
| `internal/session/session.go` | `AttachmentMeta` no evento `user` |
