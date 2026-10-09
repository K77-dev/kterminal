# Review — Task 6.0: Testes de integração E2E, regressão completa e dogfooding documentado

**Veredito: APROVADO** (com 3 notas documentadas — ver "Notas e divergências")

## Contexto da execução

Uma tentativa anterior desta task falhou por timeout de infraestrutura após já ter escrito os testes de integração (6.1/6.2). Esta execução: (1) verificou o trabalho parcial contra a task e a techspec — leitura crítica de cada teste, conferência do math de tokens/custos, execução isolada e com `-count=3` para flakiness; (2) completou o que faltava: o relatório de dogfooding (6.4) e a regressão completa documentada (6.3). Nenhum código de produção foi alterado — correto para uma task de fechamento de ciclo.

## Escopo implementado

- `internal/agent/agent_test.go` (trabalho parcial aproveitado): `TestSquadResumeLoadLatestRestoresMesa` — integração de resume ponta-a-ponta na camada agent.
- `internal/session/session_test.go` (trabalho parcial aproveitado): `TestResumeLoadLatestPreFeatureSnapshotWithoutMesa` — snapshot pré-feature via `LoadLatest` no diretório real de sessões.
- `internal/tui/tui_test.go` (trabalho parcial aproveitado): `TestResumeContinueRebuildsSquadPanel`, `TestResumePreFeatureSnapshotRendersMesaNotStarted`, `TestSquadFullFlowModeTogglePreservesMesa`, `TestSquadEscMidConvocationPanelReflectsFinalState` + helpers (`pumpUntilEvent`, `mesaEntryStatus`, `lastSnapshotMesa`, `newLiveSquadModel`, `squadBlockGateway`).
- `spec/tasks/014-prd-sidebar-squad/dogfooding.md` (**novo, desta execução**): relatório de dogfooding em pt-BR com validação programática + protocolo manual.
- Nada além do escopo: nenhuma alteração em código de produção, nenhum teste de unidade novo (N/A conforme a task), tasks.md intocado (marcação fica com o orquestrador).

## Evidências do diff

### 6.1 — Integração de resume (REQ-006)

- **`TestSquadResumeLoadLatestRestoresMesa`** (agent): turno squad real com mock gateway (kickoff architect → 1 convocation → convergência) gravado em session writer → `sess.Close()` → `LoadLatest()` devolve o path exato, modo `squad` e `Snapshot.Mesa` deep equal à mesa viva (architect `done`, 63 tokens, 1 convocation) → `AppendWriter` + segundo agent com `SetMessages` + `RestoreMesa` → `Mesa()` deep equal ao snapshot → turno de follow-up → reload: entries/roles/tetos preservados, contadores zerados para o novo turno (0 convocations / 11 tokens). É o caminho exato do `--continue` em `main.go` (`resolveSession` → `LoadLatest` → `AppendWriter`).
- **`TestResumeContinueRebuildsSquadPanel`** (tui): fluxo TUI completo com gateway + session writer real (`XDG_DATA_HOME` isolado) → `LoadLatest` → **conferência JSONL × snapshot × painel**: extrai a última linha `snapshot` do JSONL e confere `tokens`/`convocations`/`max_convocations`/`token_budget`/`entries` contra o `Snapshot` carregado; reconstrói agent + TUI na ordem do wiring do `main.go` (`ActivateMode` → `RestoreMesa` → `New` + `WithResumed`) e confere o painel renderizado com valores derivados do JSONL (`architect · done`, modelo, `12 tok`, custo, `1/4 convocations`, `63/100.0k tokens`); chat com o histórico; sem `mesa not started`; largura do view = 200−chatInset.
- **Snapshot pré-feature**: `TestResumeLoadLatestPreFeatureSnapshotWithoutMesa` (session) prova que JSONL hand-written sem `mesa` (com `mode: squad`) carrega via `LoadLatest` sem erro com `Mesa == nil`; `TestResumePreFeatureSnapshotRendersMesaNotStarted` (tui) prova o resume completo renderizando `mesa not started` com header `maestro`, **sem rodapé de orçamento** (nenhum `convocations`/`/` no painel), chat legado intacto e largura correta.

### 6.2 — Fluxo completo com mock gateway (REQ-001/002/003/004)

- **`TestSquadFullFlowModeTogglePreservesMesa`** executa a sequência exata da subtarefa: kickoff (architect+backend, tetos 4/100000) → 2 convocações → convergência (painel `architect · done`, `backend · done`, `2/4 convocations`, `90/100.0k tokens`; mesa 90/2/2 entries) → `/mode` via popup real (`openModePopup` + Up + Enter, o único caminho de troca da TUI) → **sdd**: `maestro` ausente do view, `vp.Width` em largura total, mesa intacta no agent (90/2/2) → `/mode` → **squad**: sidebar reaparece, painel preservado → **novo kickoff** (qa sozinho, tetos 2/50000): painel `qa · waiting`, `0/2 convocations`, `36/50.0k tokens`, `architect`/`backend` ausentes (sem vazamento entre modos/mesas), roles `[qa]`, contadores zerados. Math conferido: 15 (kickoff) + 15+12 (convocation 1) + 15+12 (convocation 2) + 21 (convergência) = 90; segundo turno 15+21 = 36.
- **`TestSquadEscMidConvocationPanelReflectsFinalState`**: gateway bloqueia o stream da persona mid-convocation → painel `architect · deliberating` / `1/4 convocations` → `Esc` → `turn_aborted` depth 0 → persona vira `done` (poll com deadline de 5s) → painel final `architect · done`, `1/4 convocations`, `30/100.0k tokens` (15 do kickoff + 15 da chamada de convocation; o subagente abortado não contribui — stream cancelado sem usage). `busy=false` e largura conferidos.
- Cobertura viva adicional (task 5, citada no dogfooding): `TestSquadFlowSidebarIntegration` (gates congelando o agent; painel conferido pós-kickoff/mid-convocation/pós-contribuição/convergência; largura em todos os frames).

### 6.3 — Regressão completa

Executada nesta sessão, tudo verde (tabela em "Checks executados").

### 6.4 — Dogfooding documentado

`dogfooding.md` (pt-BR) na pasta da feature, com:

1. **Validação programática executada**: tabela teste → camada → o que valida → valores conferidos, cobrindo resume (4 testes), fluxo completo/toggle/Esc (3 testes), conferência sidebar × JSONL (3 testes, incluindo a garantia por construção via espelho nos pontos de mutação) e a regressão.
2. **Protocolo manual** com passos exatos e valores esperados para o mantenedor: Cenário A (mesa real + resize ao vivo 80/120/200, incluindo resize durante convocações), Cenário B (Esc mid-convocation + conferência do snapshot no JSONL), Cenário C (resume `--continue` com mesa encerrada + caso pré-feature via `--session`), Cenário D (conferência campo-a-campo JSONL × painel, com tabela de formatos e a nota da defasagem mid-convocation), Cenário E (toggle de modos com mesa ativa + resize vertical — incorpora as recomendações da review_5.0).
3. **Valores de referência**: tabela de largura do sidebar (80→oculto, 100→24, 120→29, 140→35, 200→40) e textos canônicos do painel.
4. **Status**: matriz cenário → validação programática ✅ vs. protocolo manual pendente; a única linha sem validação programática é "mesa real com LLM de verdade (Jev roteando)", que exige terminal interativo.

## Checks executados

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio |
| `go test ./... -race -count=1` | ✅ ok (14 pacotes) |
| Testes novos isolados (`agent`/`session`/`tui`, `-race`) | ✅ todos PASS |
| `go test ./internal/tui/ -race -count=3` (integração novos + `TestSquadFlowSidebarIntegration`) | ✅ ok (flakiness) |

## Notas e divergências documentadas

1. **Dogfooding manual substituído por protocolo + validação programática**: a task pede "mesa real no próprio kterminal" com resize ao vivo e Esc — intrinsecamente interativo. Como agent headless, segui a instrução do orquestrador: protocolo documentado com passos exatos/valores esperados + validação programática de tudo que é validável (a conferência sidebar × JSONL — critério central do PRD REQ-004 — é verificada por teste com valores exatos, não apenas descrita). A execução manual dos Cenários A–E fica pendente para o mantenedor; os valores esperados do protocolo derivam do código implementado e dos valores assertados nos testes, minimizando ambiguidade. Não bloqueia os critérios da task (que exigem o protocolo documentado + validação programática neste contexto).
2. **Trabalho parcial aproveitado da tentativa anterior**: os testes de integração 6.1/6.2 já estavam escritos quando esta execução começou. Em vez de reescrever, verifiquei item a item (adesão à techspec, math de tokens/custos, determinismo, ausência de race — `-race` + `-count=3`), corrigi nada (nada precisou) e completei o delta faltante (dogfooding.md + regressão). `TestResumeLoadLatestTranscriptWithMesa` (session) é da task 2 e aparece no relatório como cobertura adicional, não como delta desta task.
3. **Observação informativa — asserções de tipo em `TestResumeContinueRebuildsSquadPanel`**: `mesaRaw["tokens"].(float64)` etc. sem check de presença; se o JSONL omitisse a chave (`omitempty` com valor zero), o teste panicaria em vez de falhar com mensagem. Para o fixture concreto (mesa com 63 tokens) é determinístico e o panic seria sinal de bug real (mesa sem tokens em sessão que consumiu tokens); não alterado para não tocar código verde sem necessidade.

## Pontos positivos

- Os testes de integração exercitam os caminhos reais de produção: `LoadLatest`/`AppendWriter` (o código exato do `--continue`), popup `/mode` via teclas (o único caminho de troca de modo), `Esc` cancelando o turno de verdade — sem atalhos de teste.
- A conferência sidebar × JSONL é tripla (JSONL ↔ Snapshot ↔ painel renderizado) e usa valores exatos (63 tokens, 12 tok da persona, custo com 4 decimais, `1/4 convocations`, `63/100.0k tokens`) — o critério "sidebar bate com o transcript" do PRD fica provado, não assumido.
- O protocolo manual incorpora as duas recomendações da review_5.0 (toggle nos dois sentidos com mesa ativa; resize vertical com mesa cheia e a limitação conhecida de altura) e documenta a defasagem mid-convocation para o mantenedor não interpretá-la como bug.
- Zero alteração em código de produção: a task fecha o ciclo sem scope creep.
