# Relatório de Code Review — Sidebar de agentes do squad (Task 6.0: Testes de integração E2E, regressão e dogfooding)

## Resumo

- Data: 2026-10-09
- Branch: `014-prd-sidebar-squad` (working tree não commitado, topo `355f714`)
- Status: **APROVADO COM RESSALVAS**
- Revisão independente (não confia no auto-review `review_6.md`; cada teste foi relido, o math de tokens reconferido e os checks reexecutados)
- Escopo da task: ~514 linhas de testes novos (agent 75 / session 28 / tui 411, incluindo helpers) + `dogfooding.md` (143 linhas)
- Diff total da feature no working tree: 10 arquivos, +2194/−36 (tasks 1–5 já revisadas; nenhum código de produção alterado nesta task — correto para task de fechamento)

## Verificação das Subtarefas

| Subtarefa | Status | Evidência independente |
| --- | --- | --- |
| 6.1 Resume reconstrói a mesa; snapshot pré-feature carrega sem erro | COMPLETA | `TestSquadResumeLoadLatestRestoresMesa` (agent_test.go:5328) percorre o caminho exato do `--continue` (`LoadLatest` → `AppendWriter` → `SetMessages` → `RestoreMesa` — mesma ordem de main.go:105-118) com deep equal snapshot × mesa viva e follow-up preservando entries/roles/tetos com contadores zerados (0/11). `TestResumeContinueRebuildsSquadPanel` (tui_test.go:3268) faz a tríade JSONL × Snapshot × painel renderizado. `TestResumeLoadLatestPreFeatureSnapshotWithoutMesa` (session_test.go:619) + `TestResumePreFeatureSnapshotRendersMesaNotStarted` (tui_test.go:3371) provam o caso pré-feature: `Mesa == nil`, `mesa not started`, sem rodapé de orçamento, chat legado intacto |
| 6.2 Fluxo completo com mock gateway, sem vazamento entre modos | COMPLETA | `TestSquadFullFlowModeTogglePreservesMesa` (tui_test.go:3426) executa a sequência exata da subtarefa via popup `/mode` real (teclas, não atalho de teste): convergência (90/100.0k, 2/4) → sdd (sidebar some, `vp.Width` full, mesa intacta no agent) → squad (painel preservado) → rekickoff `qa` (tetos 2/50000, contadores zerados, **nenhuma persona antiga vaza** — assert negativo em tui_test.go:3501). `TestSquadEscMidConvocationPanelReflectsFinalState` (tui_test.go:3558) cobre Esc mid-convocation → `done` com poll/deadline de 5s |
| 6.3 Regressão completa verde com `-race` | COMPLETA | Reexecutada por esta revisão: `go build ./...` ✅, `go vet ./...` ✅, `gofmt -l .` ✅ vazio, `go test ./... -race -count=1` ✅ (14 pacotes ok) |
| 6.4 Dogfooding documentado | COMPLETA (com ressalva) | `dogfooding.md` com validação programática (tabelas teste → camada → valores), protocolo manual com passos exatos e valores esperados (Cenários A–E), valores de referência de largura (fórmula conferida: 120→29, 140→35, 200→51 clampado a 40) e matriz de status honesta. Ressalva: execução manual dos cenários fica pendente para o mantenedor (ver desvio a) |

## Verificação Independente do Math de Tokens

Reconferido a partir dos mocks (`toolCallChunks` = 10+5 = 15 tokens; `contentChunks(p, c)` = p+c):

- Resume: kickoff 15 + convocation 15 + contribuição 12 + convergência 21 = **63** ✓ (agent_test.go:5345); follow-up 10+1 = **11** ✓ (agent_test.go:5399)
- Fluxo completo: 15+15+12+15+12+21 = **90** ✓ (tui_test.go:3446, 3452); segundo turno 15+21 = **36** ✓ (tui_test.go:3504, 3516)
- Esc abort: 15+15 = **30** ✓ (tui_test.go:3601) — o subagente bloqueado não contribui (stream cancelado sem chunk de usage, gateway espera `r.Context().Done()`)

## Veredito sobre os Desvios Declarados

| Desvio | Veredito | Fundamento |
| --- | --- | --- |
| (a) Dogfooding manual → protocolo documentado + validação programática | **ACEITO, com ressalva** | Agent headless não executa TUI interativa com LLM real. O critério central do PRD (REQ-004: "valores do sidebar batem com o transcript JSONL") não fica apenas descrito: é provado por teste com valores exatos e tripla conferência (JSONL ↔ Snapshot ↔ painel). O protocolo manual é preciso (passos + valores esperados derivados dos valores assertados). **Ressalva**: a execução dos Cenários A–E com mesa real (Jev roteando) permanece pendente — deve ser executada pelo mantenedor antes da liberação da feature |
| (b) Aproveitamento do trabalho parcial da tentativa com timeout | **ACEITO** | Verifiquei item a item de forma independente: leitura linha a linha dos 6 testes, math reconferido (seção acima), aderência à techspec confirmada (caminhos reais de produção: `LoadLatest`/`AppendWriter`, popup `/mode` via teclas, Esc real), determinismo e ausência de race com `-race -count=3` (tui 3/3 PASS em todos; agent+session 6/6 PASS). Nada precisou correção; os testes estão íntegros e completos |

## Verificação Holística de Fechamento (tasks 1–6 × REQs)

| REQ | Cobertura | Evidência (task) |
| --- | --- | --- |
| REQ-001 Exibição condicionada ao modo | COMPLETA | Toggle bidirecional sem restart e sem vazamento (6.2); matriz de visibilidade sdd/nunca, <100 ausente, ≥100 presente (4); sdd sem perda de largura conferida em 6.2 (`vp.Width` full) |
| REQ-002 Painel da mesa | COMPLETA | Nome + status textuais, cor nunca única (4: `TestSidebarStatusAlwaysTextual`); maestro no topo; `mesa not started` explícito (4 + 6.1); estado final pós-convergência e pós-Esc (6.1/6.2) |
| REQ-003 Atividade ao vivo | COMPLETA | Atividade transiente via eventos aninhados (5: `TestSquadFlowSidebarIntegration`); elisão com truncagem (4: `TestSidebarViewTruncatesLongContent`); `deliberating` ao vivo em mid-convocation (6.2) |
| REQ-004 Métricas por persona e orçamento | COMPLETA | Igualdade por construção (espelho nos pontos de mutação, 3: `TestSquadMesaFullCycle`) + verificação ponta-a-ponta JSONL × painel com valores exatos (6.1: `TestResumeContinueRebuildsSquadPanel`); rodapé n/max e k/budget |
| REQ-005 Layout responsivo | COMPLETA (programática) | Clamp piso 24/teto 40 (4: `TestSidebarWidthMatrix`); resize 80→120→200 mantém painéis (5: `TestSidebarResizeKeepsPanelsIntact`); largura conferida em todos os frames dos testes de integração (6). Execução ao vivo em terminal real: protocolo pendente (mesma ressalva do desvio a) |
| REQ-006 Persistência e resume | COMPLETA | Snapshot aditivo com round-trip (2); resume reconstruindo personas/status/métricas com deep equal (6.1, camadas agent e TUI); snapshot pré-feature carrega sem erro e exibe mesa não iniciada (6.1) |

**Lacunas bloqueantes: nenhuma.** A única lacuna não programática é a execução manual do protocolo de dogfooding com LLM real — documentada e atribuída ao mantenedor.

## Conformidade com Rules

| Rule | Status | Observações |
| --- | --- | --- |
| Sem comentários no código | OK | Nenhum comentário nas linhas adicionadas dos testes (grep no diff: vazio) |
| Eventos do agente via canal | OK | Testes consomem `ag.Events` com pumps/deadlines (`pumpUntilEvent`, `waitForEventMatch`) |
| Receivers por valor na TUI | OK | `sidebarWidth`/`sidebarView` por valor, consistente com o código-base |
| `strings.Builder` por ponteiro | OK | Helpers seguem o padrão (`writeSidebarLine(b *strings.Builder, ...)`) |
| Identificadores/textos em inglês; spec em pt-BR | OK | Testes em inglês; `dogfooding.md` em pt-BR |
| Sem dependências novas | OK | Apenas stdlib + deps já usadas (`lipgloss`, `httptest`) |
| Tratamento de erro | OK | `t.Fatalf` com contexto em todos os caminhos de falha (ver Ressalva 2 para type assertions) |
| Segurança | N/A | Funcionalidade local de TUI; sem endpoints, sem input externo não confiável, sem secrets (chaves de teste são strings de mock evidentes) |

## Aderência à TechSpec (seção "Abordagem de Testes")

| Decisão da spec | Implementado | Observações |
| --- | --- | --- |
| Integração de resume: mesa encerrada → `--continue` reconstrói; snapshot pré-feature → "mesa não iniciada" | SIM | Nas duas camadas (agent e TUI), com o wiring exato de `main.go` |
| Integração de fluxo completo: kickoff → convocações → convergência → toggle → rekickoff | SIM | Sequência exata da spec, incluindo preservação no toggle e reconstrução sem vazamento |
| E2E = dogfooding documentado (sem TestSprite, precedente 012) | SIM | `dogfooding.md` na pasta da feature; execução manual pendente (ressalva) |
| Testes usam mocks existentes de `agent_test.go`/gateway | SIM | Reuso de `taskFlowGateway`, `contentChunks`, `toolCallChunks`, `newSquadAgent`; novos helpers (`squadBlockGateway`, `newLiveSquadModel`) seguem o mesmo padrão |

## Testes

- Regressão completa: `go test ./... -race -count=1` — **14 pacotes ok, 0 falhas**
- Testes novos da task (6): todos PASS isolados e com `-race`
- Estabilidade: `go test ./internal/tui/ -race -count=3` nos 4 testes novos + `TestSquadFlowSidebarIntegration` — **3/3 PASS cada**; agent+session `-race -count=3` — **6/6 PASS**
- Significância: os testes exercitam caminhos reais de produção (sem atalhos): `LoadLatest`/`AppendWriter` (código exato do `--continue`), popup `/mode` via teclas (único caminho de troca), `Esc` cancelando turno de verdade, gateway bloqueando stream até `Context().Done()`
- Edge cases cobertos: snapshot pré-feature (retrocompatibilidade), abort mid-convocation (estado final `done`), persona sem tools (atividade vazia → sem entry em `personaActivity`), rekickoff com roles disjuntos (não-vazamento), contadores zerados no novo turno
- Coverage: aditiva — ~514 linhas de teste novo sem tocar código de produção

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
| --- | --- | --- | --- | --- |
| Baixa | internal/tui/tui_test.go | 3341-3349 | Type assertions sem comma-ok em `mesaRaw["tokens"].(float64)` etc. — se o JSONL omitisse a chave (`omitempty` com valor zero), o teste pânica em vez de falhar com mensagem | Usar `v, ok := mesaRaw["tokens"].(float64); if !ok { t.Fatalf(...) }` (informativo; determinístico no fixture concreto e o panic sinalizaria bug real) |
| Baixa | spec/tasks/014-prd-sidebar-squad/dogfooding.md | §2 | Execução manual dos Cenários A–E (mesa real, resize ao vivo, Jev roteando) pendente — única parcela do dogfooding não validável por agent headless | Mantenedor executar o protocolo antes da liberação; registrar resultado no próprio arquivo |

## Pontos Positivos

- A conferência sidebar × JSONL é tripla (JSONL ↔ Snapshot ↔ painel) com valores exatos e derivados do próprio JSONL — o critério REQ-004 fica provado, não assumido
- Testes de integração usam exclusivamente caminhos de produção: sem mocks de conveniência que contornem o código real (`/mode` via popup + teclas, `Esc` real, `LoadLatest` real com `XDG_DATA_HOME` isolado)
- Asserts negativos fortes: persona antiga não vaza no rekickoff, `mesa not started` ausente no resume com mesa, rodapé ausente na mesa pré-feature, `personaActivity` vazio para persona sem tools
- `dogfooding.md` é honesto: matriz de status separa o que foi validado programaticamente do que exige terminal interativo, e documenta a defasagem mid-convocation para o mantenedor não interpretá-la como bug
- Zero alteração em código de produção na task de fechamento — sem scope creep

## Recomendações

1. Executar o protocolo manual (Cenários A–E) antes de liberar a feature e anotar o resultado em `dogfooding.md` (rastreabilidade do critério de sucesso do PRD com LLM real)
2. Endurecer as type assertions de `TestResumeContinueRebuildsSquadPanel` com comma-ok em toque futuro no arquivo (não vale quebrar código verde agora)
3. Considerar fast-follow registrado na review 5.0 (clamp de altura do painel em terminais muito baixos) — fora do escopo do PRD, já documentado

## Conclusão

**APROVADO COM RESSALVAS.** A task 6.0 fecha o ciclo da feature 014 com qualidade alta: os testes de integração 6.1/6.2 exercitam os caminhos reais de produção com math de tokens exato (reconferido de forma independente: 63/90/36/30), a regressão completa está verde com `-race` (reexecutada por esta revisão) e os testes novos são estáveis em `-count=3`. Os dois desvios declarados são legítimos: o aproveitamento do trabalho parcial foi verificado item a item de forma independente e está íntegro; a substituição do dogfooding manual por protocolo + validação programática é a única opção viável para agent headless e a parcela central do critério (sidebar × JSONL) ficou provada por teste. As ressalvas são não-bloqueantes e rastreadas: (1) execução manual dos Cenários A–E pendente para o mantenedor; (2) robustez menor de type assertions num único teste. A verificação holística confirma que a feature completa (tasks 1–6) cobre todos os seis REQs do PRD sem lacunas bloqueantes.

## Checks Executados (por esta revisão)

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio |
| `go test ./... -race -count=1` | ✅ ok (14 pacotes) |
| Testes novos tui `-race -count=3` (4 novos + `TestSquadFlowSidebarIntegration`) | ✅ 3/3 PASS cada |
| Testes novos agent+session `-race -count=3` | ✅ 6/6 PASS |
