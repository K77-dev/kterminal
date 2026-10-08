# Tech Spec — Diagnósticos pós-edição (go vet hook)

## Requisitos Atendidos

- REQ-001 — Hook pós-edição em arquivos Go
- REQ-002 — Diagnóstico best-effort com `go vet`
- REQ-003 — Feedback explícito ao modelo
- REQ-004 — Correção autônoma no mesmo turno

## Resumo Executivo

O `Registry` ganha um hook injetável `OnGoEdit func(path string) string`, chamado após `write`/`edit` bem-sucedidos em arquivos `.go` dentro de um módulo Go (detectado pela presença de `go.mod` subindo diretórios). A implementação em `internal/tools/diagnostics.go` roda `go vet` no pacote do arquivo editado com timeout de 30s, filtra a saída para o arquivo editado e devolve um bloco `Diagnostics:` — com as linhas de erro relevantes, ou `Diagnostics: clean` como feedback explícito de sucesso. O bloco é anexado ao `Output` da tool, que volta ao LLM como mensagem `role: "tool"` pelo fluxo existente — o modelo reage ao texto e corrige o erro no passo seguinte, fechando o loop sem intervenção humana. O hook é best-effort por contrato: Go ausente no PATH, falha de rede ou timeout retornam string vazia — **nenhum erro de diagnóstico pode falhar a tool em si** (requisito não negociável). A injeção acontece no `main.go`, mantendo o pacote `tools` testável sem Go instalado.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/tools/diagnostics.go`** (novo): `GoVetHook(path string) string` — detecção de módulo, execução de `go vet`, filtragem, formatação; `goModuleRoot(path) (root, rel string, bool)`.
- **`internal/tools/tools.go`** (modificado): campo `OnGoEdit func(path string) string` no `Registry` (nil default); `Execute` chama o hook após sucesso de `write`/`edit` em `.go`.
- **`main.go`** (modificado): `reg.OnGoEdit = tools.GoVetHook` na inicialização.
- **`internal/agent`** (sem mudança): o resultado com diagnóstico circula como `role: "tool"` pelo fluxo existente — REQ-004 é consequência, não código novo.
- **`internal/tui`** (sem mudança): o diagnóstico aparece no `EventToolResult` como qualquer output, com truncamento normal.
- **`internal/session`** (sem mudança): o diagnóstico já vai no `Result` do evento `tool_result`.

Fluxo de dados:

1. `write`/`edit` em `.go` bem-sucedido → `Registry.Execute` extrai `path` dos args → `OnGoEdit(path)`.
2. `GoVetHook`: acha `go.mod` (senão → `""`) → `go vet ./<pacote>` no root do módulo (timeout 30s) → filtra linhas do arquivo → formata bloco.
3. `Output = output + bloco` → `EventToolResult` (TUI + transcript) → `llm.Message{Role: "tool"}` → o modelo corrige no passo seguinte.

## Design de Implementação

### Interfaces Principais

```go
const vetTimeout = 30 * time.Second

func (r *Registry) SetOnGoEdit(hook func(path string) string)

func GoVetHook(path string) string

func goModuleRoot(path string) (root, rel string, ok bool)
```

Regras do `Registry.Execute` (ponto de chamada):

- Após sucesso da tool, se `name == "write" || name == "edit"`, `r.OnGoEdit != nil`, e `strings.HasSuffix(path, ".go")` → `out += r.OnGoEdit(path)`.
- O hook roda **após** a escrita ter sucesso — a edição nunca é revertida nem falha por causa do diagnóstico.

Regras do `GoVetHook`:

1. `goModuleRoot`: a partir de `filepath.Abs(filepath.Dir(path))`, sobe um diretório por vez procurando `go.mod`; retorna `(root, rel do arquivo, true)`. Sem `go.mod` até a raiz → `""` (REQ-001: fora de módulo não roda).
2. `exec.LookPath("go")` falha → `""` (ambiente sem Go — REQ-002).
3. `exec.CommandContext(ctx, "go", "vet", pkgPattern)` com `ctx` de 30s, `cmd.Dir = root`, env `GOFLAGS=-mod=mod` (não travar em downloads). `pkgPattern = "./" + filepath.Dir(rel)` (normalizado; arquivo na raiz → `"./..."` é incorreto — usar `"."`).
4. Saída combinada (stdout+stderr) → filtrar linhas que contêm o caminho relativo do arquivo (`rel` com separador do OS) — diagnóstico apenas do arquivo editado, sem ruído do repositório (REQ de foco do PRD).
5. Formatação: linhas filtradas → `"\n\nDiagnostics:\n" + linhas`; nenhuma linha → `"\n\nDiagnostics: clean"` (REQ-003 — feedback explícito também no sucesso).
6. Qualquer erro do comando (exit status, timeout, rede) **sem** linhas filtradas → se timeout: matar via contexto e retornar `""`; se exit status com linhas do arquivo: retornar as linhas (é o caso normal de diagnóstico); se exit status sem linhas do arquivo (erro de ambiente/build de outro pacote) → `""`.

### Modelos de Dados

- Sem mudanças de struct — o diagnóstico é texto no `Output`/`Result`.
- Formato do bloco (contrato com o modelo):

```
wrote 120 bytes to internal/foo/bar.go

Diagnostics:
internal/foo/bar.go:12:6: undefined: Foo
```

ou

```
edited internal/foo/bar.go

Diagnostics: clean
```

## Pontos de Integração

- **`go` binary do ambiente**: única dependência externa — opcional e best-effort (`LookPath` guard). Timeout de 30s via `context.WithTimeout`; processo morto pelo `CommandContext` no estouro.
- **Rede do `go vet`**: pode baixar dependências (`GOFLAGS=-mod=mod`); falha de rede → exit status sem linhas do arquivo → `""` silencioso.
- **Fluxo do agente**: zero mudanças — o texto anexado circula como qualquer tool result.

## Verificações Técnicas

### Segurança

- `go vet` roda no diretório do módulo do arquivo editado — código do próprio usuário, mesmo nível de confiança do tool `bash` existente.
- `GOFLAGS=-mod=mod` pode alterar `go.mod`/`go.sum` como efeito colateral do build de diagnóstico — trade-off aceito (alternativa `-mod=readonly` falharia o vet em módulos desatualizados, piorando o best-effort); documentado como comportamento conhecido.
- Nenhum input do modelo chega à linha de comando do `go` (o path vem dos args da tool, já validados; `pkgPattern` é derivado de caminho, não de texto livre do LLM).

### Arquitetura

- Hook injetado via setter (não no `NewRegistry`): o pacote `tools` e seus testes não dependem de Go instalado — requisito do PRD; o `main.go` é o ponto de composição.
- Best-effort como contrato: `GoVetHook` **nunca** retorna erro — só string (possivelmente vazia); a tool não conhece o conceito de falha de diagnóstico.
- Custo: +1 execução de `go vet` por edição em `.go` — típico 1-5s em módulo com cache quente; timeout de 30s limita o pior caso (edição não espera mais que isso).
- Foco no arquivo editado: filtro por caminho relativo evita que erros pré-existentes de outros arquivos virem ruído para o modelo.

### Infraestrutura

- Sem novos requisitos — stdlib (`os/exec`, `context`, `path/filepath`, `strings`). Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/tools/diagnostics_test.go`** (temp dirs; casos que exigem `go` pulados com `exec.LookPath` guard + `t.Skip`):

1. `TestGoModuleRootFound` — temp dir com `go.mod` + `internal/x/a.go` → root e rel corretos.
2. `TestGoModuleRootNotFound` — arquivo fora de módulo → ok=false.
3. `TestVetHookReturnsDiagnostic` (mandatório): temp module com `go.mod` + arquivo com `undefined: Foo` → hook retorna bloco `Diagnostics:` contendo a linha com o caminho relativo.
4. `TestVetHookCleanOnValidFile` — arquivo válido → `Diagnostics: clean`.
5. `TestVetHookNoModuleEmpty` — sem `go.mod` → `""` (não roda vet).
6. `TestVetHookFiltersOtherFiles` — módulo com erro em `other.go`, arquivo editado válido → `Diagnostics: clean` (erro de outro arquivo filtrado).
7. `TestVetHookTimeoutKillsProcess` — módulo com teste que demora (ou `go vet` substituído por script lento via env `PATH` do teste) → retorna `""` dentro do timeout + margem.
8. `TestVetHookNoGoInPath` — `PATH` vazio no env do cmd → `""` imediato.

**`internal/tools/tools_test.go`**:

9. `TestExecuteCallsHookOnGoEdit` — registry com hook fake + `write` em `.go` → output contém o retorno do hook; hook registra o path recebido.
10. `TestExecuteSkipsHookForNonGoOrOtherTools` — `write` em `.txt`, `edit` em `.md`, `read`/`bash` → hook não chamado.
11. `TestExecuteNilHookNoop` — registry sem hook → comportamento idêntico ao atual.

**`internal/agent/agent_test.go`**:

12. `TestAgentSelfCorrectsWithinTurn` (REQ-004, mandatório): mock gateway — 1ª resposta chama `edit` que quebra (hook fake injetado retorna `Diagnostics:\n...undefined: Foo`); o mock captura a 2ª chamada: a mensagem `role: "tool"` contém o diagnóstico; 2ª resposta do mock fecha o turno — loop completo sem input do usuário.

### Testes de Integração

Cobertos por 12 (agent + registry + hook fake, fluxo ponta a ponta do turno).

### Testes de E2E

Deferidos para `kspec-qa`: edição real que quebra código → diagnóstico visível no resultado da tool → agente corrige sozinho no mesmo turno; edição válida → `Diagnostics: clean`; edição fora de módulo Go → sem diagnóstico.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/tools/diagnostics.go`**: `goModuleRoot` + `GoVetHook` + testes com temp modules — isolado e testável.
2. **`internal/tools/tools.go`**: campo `OnGoEdit` + chamada em `Execute` + testes de hook fake.
3. **`internal/agent`**: teste de loop de auto-correção (12) — valida o requisito sem código novo.
4. **`main.go`**: injeção `reg.OnGoEdit = tools.GoVetHook`.
5. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib.
- Interação com techspec 002: o hook anexa ao `Output` do `Result` (assinatura pós-002); se 002 ainda não implementado, anexa ao `string` — mudança mecânica.
- Interação com techspec 007: se `ExecuteStream` existir, o hook roda no fluxo consolidado (após o fim da execução), independente de `onLine`.

## Monitoramento e Observabilidade

### Error Tracking

O diagnóstico nunca é erro: falhas do hook são silenciosas por contrato (string vazia). Erros da edição em si seguem o fluxo existente.

### Logging Estruturado

Transcript JSONL: o diagnóstico já viaja no `Result` do evento `tool_result` — auditoria natural do que o modelo viu e quando se auto-corrigiu. Sem campo novo.

### Health Checks / Métricas / Alertas

Não aplicável — TUI local. Sinal indireto de eficácia: turnos com `Diagnostics:` seguidos de correção no transcript.

## Considerações Técnicas

### Decisões Principais

1. **Hook como campo injetável no `Registry`** (PRD): pacote `tools` testável sem Go instalado; `main.go` compõe — padrão de injeção explícito, sem global.
2. **`Diagnostics: clean` explícito** (REQ-003): silêncio é ambíguo para o modelo (erro de ambiente vs. sucesso); o feedback positivo fecha o loop de confiança — custo de ~20 tokens por edição.
3. **Filtro por caminho relativo do arquivo**: o PRD exige foco — erros pré-existentes de outros arquivos não viram ruído; o modelo só vê o que a edição dele causou.
4. **`go vet` puro, sem LSP** (PRD): retorno imediato no mesmo turno; LSP completo é evolução futura com infra pesada.
5. **`GOFLAGS=-mod=mod`**: prioriza o diagnóstico funcionar em módulos desatualizados; efeito colateral em `go.mod`/`go.sum` aceito e documentado.

### Riscos Conhecidos

- **Latência do primeiro vet** (cache frio: downloads de dependências): timeout de 30s limita; falha vira silêncio — o loop de edição nunca bloqueia indefinidamente.
- **Falsos negativos**: `go vet` não pega todos os erros de compilação (ex.: erros de tipo em pacotes não importados pelo pacote editado) — aceito; é um primeiro passo, não um compilador.
- **Arquivo editado fora de qualquer pacote compilável** (ex.: build tags): vet ignora → `clean` — comportamento aceitável.
- **Concorrência**: `go vet` roda sincronamente no passo da tool — sem corrida (o loop do agent é sequencial por passo).

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing`, temp dirs, skips condicionais para binário ausente).
- Padrões do projeto: sem comentários no código, sem dependências externas, tools via registry.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (auto-correção visível no mesmo turno), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/tools/diagnostics.go` | Novo — `GoVetHook`, `goModuleRoot`, filtro e formatação |
| `internal/tools/diagnostics_test.go` | Novo — módulos temporários, diagnóstico/clean/timeout/sem-go |
| `internal/tools/tools.go` | Campo `OnGoEdit` + chamada pós `write`/`edit` em `.go` |
| `internal/tools/tools_test.go` | Hook chamado/não chamado, nil noop |
| `internal/agent/agent_test.go` | Teste de auto-correção no mesmo turno (REQ-004) |
| `main.go` | Injeção do hook na inicialização |
| `internal/agent/agent.go` | Sem mudança — fluxo existente carrega o diagnóstico |
| `internal/tui/tui.go` | Sem mudança — resultado renderizado como hoje |
| `internal/session/session.go` | Sem mudança — diagnóstico no `Result` existente |
