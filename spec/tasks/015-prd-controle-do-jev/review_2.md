# Review — Task 2.0: Router — `FixedRouter` determinístico e factory `Wire()`

**Veredito: APROVADO**

## Escopo do diff

| Arquivo | Status | Conteúdo |
| --- | --- | --- |
| `internal/router/router.go` | modificado | `FixedRouter` (struct + `Route`) e `Wire()` adicionados ao final do arquivo; `JevRouter`, `HeuristicRouter`, `Decision`, interface `Router` e `MinConfidence` intocados |
| `internal/router/fixed_test.go` | novo | 6 testes de unidade do `FixedRouter` + helper `fixedTestCatalog` (catálogo hermético via `catalog.Parse`, independente do `models.yaml` embutido) |
| `internal/router/wire_test.go` | novo | 4 testes de unidade do `Wire()` |

Nenhum arquivo fora do escopo foi tocado: `main.go` e `internal/tui/tui.go` permanecem com o bloco de wiring duplicado (a substituição por `Wire()` é das tasks 3.0 e 4.0, conforme a task 2.0). `internal/agent` e `internal/jev` sem mudanças (restrição do PRD).

## Evidências do diff × requisitos

### `FixedRouter` (subtarefa 2.1)

- Struct exatamente como na techspec: `FixedRouter{Model, Default string}` — sem campos extras.
- Assinatura exata da techspec: `func (r *FixedRouter) Route(_ context.Context, _ string, candidates []catalog.Model) (Decision, error)` — `state` ignorado, conforme especificado.
- Ordem de decisão implementada: `Model` se presente nos candidatos → `Default` se presente → `candidates[0]`; `Confidence: 1` e `Router: "fixed"` em todos os ramos; sem candidatos → `fmt.Errorf("no candidate models")` (mesma mensagem de erro do contrato usado por `JevRouter`/`HeuristicRouter`, router.go:43-44 e router.go:75-76).
- Determinismo: `Route` não lê `state` nem relógio/aleatoriedade; mesma lista de candidatos → mesma `Decision` (coberto por `TestFixedRouterDeterministicAcrossStates` com 4 estados distintos).

### `Wire()` (subtarefa 2.2)

- Assinatura: `func Wire(apiKey string, jevEnabled bool, fixedModel string, cat *catalog.Catalog) (Router, Router)`. Obs.: a techspec escreve o retorno como `(router.Router, router.Router)` na perspectiva do caller; dentro do pacote `router` o tipo é referido como `Router` (o pacote não pode qualificar o próprio identificador). Idêntico para os callers.
- Jev ativo (`jevEnabled && apiKey != ""`): primary `NewJev(jev.New(apiKey))`, fallback `&HeuristicRouter{Default: cat.DefaultModel}` — reproduz exatamente o comportamento atual de main.go:82-90 com chave presente.
- Jev inativo (enabled=false OU sem chave): primary `&FixedRouter{Model: fixedModel, Default: cat.DefaultModel}`, fallback `nil` — `fixedModel` passado verbatim (sem normalização), conforme o bloco da techspec.
- Garantia de zero chamadas ao endpoint Typesafe com Jev desabilitado: a condição composta só constrói `JevRouter` quando `jevEnabled && apiKey != ""`; `jev.New` é construtor puro (sem I/O, jev.go:50-59), então com Jev desabilitado nenhum cliente Typesafe existe na sessão — subagentes herdam `Router` do pai e ficam cobertos pela mesma garantia.
- Sem ciclos de import: `router` já importava `jev` e `catalog`; nenhum import novo em router.go.

### Testes (subtarefas 2.3 e 2.4)

`fixed_test.go`:

- `TestFixedRouterPicksConfiguredModel` — Model presente nos candidatos → escolhe Model, Confiança 1, Router "fixed".
- `TestFixedRouterDegradesToDefault` — Model ausente → degrada para Default (com Confiança 1 e Router "fixed").
- `TestFixedRouterDegradesToFirstCandidate` — Model e Default ausentes → `candidates[0]`.
- `TestFixedRouterVisionFilterKeepsChainDeterministic` — candidatos filtrados por visão (fixo não-vision excluído): cadeia determinística preservada nos dois caminhos — Default também não-vision → primeiro candidato vision; Default vision presente → Default (não o primeiro), provando que o ramo Default dispara dentro do filtro.
- `TestFixedRouterDeterministicAcrossStates` — mesmo conjunto de candidatos, 4 estados diferentes → mesmo `Decision.Model` (critério de sucesso da task).
- `TestFixedRouterNoCandidatesError` — sem candidatos → erro.

`wire_test.go`:

- `TestWireJevActiveReturnsJevAndHeuristic` — (chave, enabled=true) → (*JevRouter, *HeuristicRouter) com `Default = cat.DefaultModel`.
- `TestWireJevDisabledNeverReturnsJevRouter` — (chave, enabled=false) → (*FixedRouter, nil), com asserção de tipo negativa de que NÃO é `*JevRouter` (critério de sucesso da task).
- `TestWireWithoutKeyReturnsFixed` — (sem chave, enabled true e false) → (*FixedRouter, nil) em ambos, nunca `*JevRouter`.
- `TestWireEmptyModelRoutesToCatalogDefault` — `fixedModel=""` → `FixedRouter` efetivo roteia para o `default_model` do catálogo (verificado via `Route`, pois `Wire` não normaliza o campo).

## Decisão documentada — `Reason` da degradação

A techspec (Riscos Conhecidos) exemplifica o `Reason` do caso de visão como `"fixed: model not vision-capable"`. O `FixedRouter` implementado usa reasons factuais por ramo: `"fixed model"`, `"fixed: model not in candidates"` e `"fixed: model and default not in candidates"`.

Racional: pela própria assinatura da techspec (`Route` ignora `state` e a struct não carrega catálogo), o `FixedRouter` não consegue distinguir a causa da exclusão do modelo fixo dos candidatos — filtro de visão (agent.go:1054) versus ausência no gateway (`Catalog.Available`). Hardcodar `"fixed: model not vision-capable"` em toda degradação produziria razão factualmente errada no caso "modelo ausente do gateway"; inferi-la de "todos os candidatos são vision" é uma suposição que pode mentir (gateway expondo apenas modelos vision) e insere adivinhação num componente cuja identidade é o determinismo. Os reasons implementados mantêm o evento rastreável no transcript (o `Reason` explica que o modelo fixo não estava entre os candidatos e a `Decision` mostra para onde roteou), que é o requisito da task — o string da techspec é introduzido por "ex:" tanto na task quanto na techspec, e nenhum critério de aceite ou teste da task o exige literal. Se o string literal for considerado mandatório no futuro, a correção natural é enriquecer o `Reason` no `agent.decide()` (que sabe `hasImages`) — mudança fora do escopo desta task (techspec: `internal/agent` sem mudanças).

## Checks executados

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ verde |
| `go vet ./...` | ✅ verde |
| `gofmt -l .` | ✅ sem output (nenhum arquivo listado) |
| `go test ./internal/router/` | ✅ ok — 11 testes (1 pré-existente + 10 novos), todos passando |
| `go test ./...` (suite completa) | ✅ ok em todos os pacotes (agent, catalog, config, kspec, llm, router, session, squad, telemetry, tools, tui, raiz) |

Baseline verificado antes das mudanças: build/vet/gofmt/test já estavam verdes na árvore (com a task 1.0 aplicada); nenhum check pré-existente foi quebrado.

## Conformidade com padrões do repo

- Sem comentários no código ✅
- Código em inglês ✅
- `JevRouter` e `HeuristicRouter` inalterados ✅
- Erro de "no candidate models" segue a mensagem existente do contrato da interface ✅
- Testes herméticos (catálogo próprio via `catalog.Parse`, sem rede, sem dependência do conteúdo do `models.yaml` embutido) ✅

## Subtarefas

- [x] 2.1 `FixedRouter` com ordem Model → Default → primeiro candidato e `Router: "fixed"`
- [x] 2.2 `Wire()` decidindo o par (primary, fallback) pelas regras da techspec
- [x] 2.3 Testes de unidade do `FixedRouter`
- [x] 2.4 Testes de unidade do `Wire()`
