# Tech Spec — @menções de arquivo no prompt

## Requisitos Atendidos

- REQ-001 — Expansão de menções
- REQ-002 — Autocomplete com popup
- REQ-003 — Proteções de expansão
- REQ-004 — Transcript fiel

## Resumo Executivo

A expansão de `@caminho` acontece **na TUI, antes do envio ao agente** — o agente nunca conhece o conceito de menções (requisito não negociável). Um novo arquivo `internal/tui/mentions.go` expõe `ExpandMentions(text) (string, []string)`: tokens `@<caminho>` válidos são expandidos em blocos `--- Arquivo @caminho ---` anexados ao texto original (limite de 64k por arquivo, igual ao tool `read`; máximo 5 arquivos; menções dentro de code fences ignoradas; arquivo inexistente permanece como texto). O popup de autocomplete é um estado novo do Model (`mentionItems`, `mentionSelected`), alimentado por glob do prefixo digitado, renderizado entre viewport e prompt box, com `Tab`/`Enter` completando e `Esc` fechando. O bloco do usuário no chat mostra só o texto original; o transcript grava a mensagem expandida (o que o modelo viu) — automático, pois o agent recebe e grava a mensagem já expandida.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/tui/mentions.go`** (novo): `ExpandMentions(text string) (expanded string, warnings []string)` e `mentionPrefix(input string, cursor int) (string, bool)` — extração do token `@...` em digitação.
- **`internal/tui/tui.go`** (modificado): estado de popup (`mentionOpen bool`, `mentionItems []string`, `mentionSelected int`); key handling novo (`Tab`, `Enter` com popup aberto, `Esc`); no envio, `ExpandMentions` antes de `agent.Run`; renderização do popup entre viewport e prompt box; aviso inline de limite.
- **`internal/agent`** (sem mudança): `Run(userInput)` recebe a mensagem já expandida — o transcript (`session.Event{Type: "user"}`) grava o expandido por construção (REQ-004).
- **`internal/tools/fs.go`** (sem mudança): o limite de 64k é duplicado como constante da TUI (`maxMentionBytes = 64 * 1024`, mesmo valor do `maxReadBytes`).

Fluxo de dados:

1. **Digitação**: cada atualização do input extrai o token `@...` sob o cursor → glob do prefixo → popup (máx. 10, ordenado).
2. **Envio**: Enter → `expanded, warnings := ExpandMentions(input)` → bloco do chat renderiza o **original** → `agent.Run(expanded)` → transcript grava o expandido.
3. **Aviso**: warnings (ex.: "max 5 file mentions") renderizados inline em `colWarning` acima do prompt, sem bloquear o envio.

## Design de Implementação

### Interfaces Principais

```go
const maxMentionFiles = 5
const maxMentionBytes = 64 * 1024
const maxMentionSuggestions = 10

var mentionRe = regexp.MustCompile(`@([^\s@]+)`)

func ExpandMentions(text string) (string, []string)

func mentionPrefix(input string) (prefix string, ok bool)
```

Regras de `ExpandMentions`:

- Percorre o texto linha a linha, rastreando blocos de código: linha iniciada por 4 espaços ou ``` alterna o modo "dentro de code fence" — menções nessas linhas são ignoradas (REQ-003).
- Fora de fences, cada match de `mentionRe`: arquivo existente e dentro do limite de 5 → lê conteúdo (trunca em `maxMentionBytes` + `... (truncated)`, igual ao tool `read`); inexistente → token permanece como texto (o agente decide).
- Saída: `<texto original com tokens intactos>` + `\n\n` + para cada arquivo expandido: `--- Arquivo @caminho ---\n<conteúdo>\n`.
- Excedentes além de 5: ficam como texto; `warnings` recebe `"max 5 file mentions"` (renderizado em `colWarning`).

Popup de autocomplete:

- `mentionPrefix(input)`: extrai o token `@...` em digitação (do último `@` sem espaço até o cursor); sem token → popup fechado.
- Sugestões: `filepath.Glob(prefix + "*")` quando o prefixo tem diretório, ou `prefix*` no cwd; arquivos apenas (não diretórios); ordenados; máximo 10.
- Teclas com popup aberto: `Tab` → completa com o item selecionado (substitui o token no input); `Enter` → completa **sem enviar** (REQ-002); `Esc` → fecha; `↑`/`↓` → movem seleção; qualquer digitação → re-glob.
- Renderização: entre viewport e prompt box; itens em `colTextMuted`, selecionado em `colPrimary`; paleta existente, sem cores novas.

### Modelos de Dados

- Estado novo no `Model` (por valor, padrão Bubble Tea): `mentionOpen bool`, `mentionItems []string`, `mentionSelected int` — sem `strings.Builder` por valor (bug conhecido do projeto).
- Sem mudanças em `agent.Event`, `session.Event` ou `llm.Message` — a expansão é transparente para tudo abaixo da TUI.

## Pontos de Integração

- **Nenhuma externa** — glob e leitura são locais (`os`/`filepath`).
- Contrato com o agente preservado: o agente vê apenas uma mensagem user mais longa; nenhum conceito de menção vaza para `internal/agent` (restrição não negociável do PRD).

## Verificações Técnicas

### Segurança

- Leitura de arquivo arbitrário pelo próprio usuário no próprio prompt — mesmo poder do tool `read` que o agente já tem; sem superfície nova.
- Limite de 64k por arquivo e 5 por mensagem contêm o tamanho do prompt (proteção de custo/estouro indireta; a compação da techspec 003 cobre o resto).

### Arquitetura

- Expansão isolada em `mentions.go` com função pura (`text → expanded, warnings`) — testável sem Bubble Tea.
- O popup não introduz estado global; vive no Model com receivers por valor.
- Direção de dependência inalterada: `tui → agent`; a TUI não importa `tools` (duplica a constante de 64k para evitar acoplamento — valor idêntico documentado).

### Infraestrutura

- Sem novos requisitos — stdlib (`regexp`, `os`, `path/filepath`, `sort`). Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/tui/mentions_test.go`** (função pura, temp dir como cwd):

1. `TestExpandExistingFile` — `@a.txt pergunta` → expandido contém o bloco `--- Arquivo @a.txt ---` + conteúdo; texto original preservado com o token.
2. `TestExpandMissingFilePassesThrough` — `@nope.txt` → saída idêntica à entrada (token intacto, sem bloco).
3. `TestExpandMultipleMentions` — 3 menções → 3 blocos na ordem de aparição.
4. `TestExpandLimitFive` — 6 menções válidas → 5 blocos + warning `"max 5 file mentions"`; a 6ª permanece como texto.
5. `TestExpandIgnoresCodeFences` — menção em linha com 4 espaços ou dentro de ``` → não expandida.
6. `TestExpandTruncatesLargeFile` — arquivo de 100k → conteúdo truncado em 64k + `... (truncated)`.
7. `TestMentionPrefixExtraction` — `"@int"` → prefix `int`; `"texto @internal/t"` → `internal/t`; sem `@` → ok=false; `@` seguido de espaço → não é token.

**`internal/tui/tui_test.go`** (harness `newTestModel`/`step`, temp dir com arquivos):

8. `TestPopupListsFilesOnAtSign` — digitar `@` → popup aberto com arquivos do cwd (máx. 10, ordenados).
9. `TestTabCompletesFirstResult` — `Tab` com popup aberto → input contém o caminho completo; popup fechado.
10. `TestEnterWithPopupCompletesNotSends` — `Enter` com popup aberto → input completado, `agent.Run` **não** chamado; `Enter` com popup fechado → envia.
11. `TestEscClosesPopup` — `Esc` fecha o popup sem cancelar turno (agente ocioso).
12. `TestSendExpandsButChatShowsOriginal` — enviar `@a.txt o que é?` → bloco do chat contém só o texto original; a mensagem recebida pelo agent (mock/capture) contém o bloco expandido.
13. `TestWarningRendersInline` — 6 menções → aviso `max 5 file mentions` visível em `colWarning`.

### Testes de Integração

`TestSendExpandsButChatShowsOriginal` cobre TUI→agent com captura do input — o contrato ponta a ponta.

### Testes de E2E

Deferidos para `kspec-qa`: `@main.go o que esse arquivo faz?` responde sobre o arquivo sem tool call de leitura; autocomplete com Tab; menção inexistente chega intacta ao agente.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/tui/mentions.go`** + testes da função pura — núcleo isolado.
2. **`internal/tui`**: expansão no envio + bloco do chat com texto original + aviso inline.
3. **`internal/tui`**: estado do popup, `mentionPrefix` no key handling, glob de sugestões.
4. **`internal/tui`**: teclas Tab/Enter/Esc/↑↓ e renderização do popup.
5. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib.
- Interação com techspec 007 (multiline): `mentionPrefix` opera sobre o texto do input multi-linha — a extração considera o cursor linha a linha; implementar 005 antes ou depois é indiferente (a função recebe o texto corrente).

## Monitoramento e Observabilidade

### Error Tracking

Menção inválida não é erro — passa intacta ao agente (design do PRD). Falha de leitura (permissão) trata o arquivo como inexistente (token permanece) — sem `EventError`.

### Logging Estruturado

Transcript JSONL: evento `user` grava a mensagem **expandida** (o que o modelo viu — REQ-004) — fidelidade de auditoria; o texto original fica recuperável pelo bloco de chat renderizado.

### Health Checks / Métricas / Alertas

Não aplicável — TUI local.

## Considerações Técnicas

### Decisões Principais

1. **Expansão na TUI, agente alheio** (requisito não negociável): zero mudanças em `internal/agent` — o modelo recebe contexto grounded sem tool calls, e o conceito de menção nunca contamina o contrato do agente.
2. **Função pura `ExpandMentions`**: testável sem Bubble Tea; determinística (cwd no momento do envio).
3. **Transcript expandido vs chat original**: o JSONL grava o que o modelo viu (auditoria fiel); o chat mostra o que o usuário digitou (leitura limpa) — ambos requisitos do PRD, resolvidos com uma única expansão no ponto de envio.
4. **Popup entre viewport e prompt**: posição do PRD; reusa paleta existente (muted/primary) — sem nova linguagem visual.
5. **Constante 64k duplicada na TUI** em vez de importar `internal/tools`: evita acoplamento TUI→tools; o valor é requisito do PRD ("limite idêntico ao do tool read").

### Riscos Conhecidos

- **Glob em repositório grande**: `prefix*` no cwd pode varrer diretórios grandes — mitigado por máximo 10 resultados e prefixo mínimo (popup abre a partir de 1 caractere após `@`).
- **Menção com `@` no meio de palavra** (ex.: `email@host`): regex `[^\s@]+` exige `@` precedido de espaço/início — implementar com boundary; testado em `TestMentionPrefixExtraction`.
- **Arquivo modificado entre popup e envio**: a leitura acontece no envio — o conteúdo é o do momento do envio; correto por definição.
- **Cursor no meio do texto**: `mentionPrefix` usa a posição do cursor do `textinput` — completar substitui o token sob o cursor, não o fim do input.

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing`, AAA, independência, temp dirs).
- Padrões do projeto: sem comentários no código, receivers por valor na TUI, `strings.Builder` por ponteiro, paleta existente.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (fluxo E2E de menção + autocomplete), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/tui/mentions.go` | Novo — `ExpandMentions`, `mentionPrefix`, constantes de limite |
| `internal/tui/mentions_test.go` | Novo — 7 testes da função pura |
| `internal/tui/tui.go` | Estado do popup, key handling (Tab/Enter/Esc/↑↓), expansão no envio, renderização popup/aviso |
| `internal/tui/tui_test.go` | Popup, teclas, fidelidade chat×transcript, aviso inline |
| `internal/agent/agent.go` | Sem mudança — recebe a mensagem expandida |
| `internal/session/session.go` | Sem mudança — evento `user` grava o expandido por construção |
| `internal/tools/fs.go` | Sem mudança — referência do limite de 64k (`maxReadBytes`) |
