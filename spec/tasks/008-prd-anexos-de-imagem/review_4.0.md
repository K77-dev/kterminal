# Relatório de Code Review - Anexos de imagem no prompt (Task 4.0: `RunWithAttachments`, roteamento por visão e transcript sem base64)

## Resumo
- Data: 2026-10-04
- Branch: working tree (baseline a2fa08f; mudanças 001–007 e 008/1.0–3.0 não commitadas preservadas — a camada desta review é apenas a task 4.0)
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 6 (`internal/agent/agent.go`, `internal/agent/agent_test.go`, `internal/session/session.go`, `internal/session/session_test.go`, `internal/tui/tui.go`, `internal/tui/tui_test.go`)
- Linhas Adicionadas: ~598 (camada 4.0: ~105 em código — `agent.go` ~30, `session.go` ~49, `tui.go` ~25; ~493 em testes — `agent_test.go` 331, `session_test.go` 54, `tui_test.go` 108)
- Linhas Removidas: ~30 (substituições: assinaturas de `RunWithAttachments`/`loop`/`decide`/`buildState`, corpo de `WriteSnapshot`, ramo de envio da TUI, handler de `EventError`)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| code-standards.md | OK | Rule vazia no repo; padrões do projeto seguidos: zero comentários no código novo (verificado por grep em fontes e testes), nomenclatura consistente, gofmt limpo |
| architecture-ddd.md | N/A | Brownfield Go com packages `internal/`, conforme techspec |
| tests.md (Vitest) / demais rules de stack | N/A | Stack Go — `testing` stdlib + `httptest`, padrão do repositório (AAA, temp dir, mocks por handler) |
| Padrões do projeto (eventos via canal, receivers) | OK | Erro de visão via `emitError` (canal + evento `error` no transcript); handlers da TUI por valor (padrão existente); mutações de estado nos handlers existentes |
| Sem comentários no código | OK | Nenhum `//` adicionado em fontes ou testes |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `RunWithAttachments(text string, parts []llm.ContentPart, meta []session.AttachmentMeta)` | SIM | Assinatura exata da techspec; resolve a Ressalva 1 da task 3.0; único call site (TUI) ajustado |
| `Run` vira wrapper com parts nil — callers inalterados | SIM | `Run` passa `nil, nil`; todos os callers de teste/produção de `Run` intactos (151 testes pré-existentes passando) |
| Filtragem em `decide` quando `hasImages`: `candidates = filter(candidates, m.Vision)` | SIM | `filterVision` aplicada após o enriquecimento de TPS medido; routers intocados (decisão 2 da techspec) — `Route` recebe a lista já filtrada |
| Vazio → `emitError("no vision-capable model available")` sem consumir a mensagem | SIM | Pre-check em `loop` **antes** do append ao histórico — ver Ressalva 1 (interpretação da cláusula "em decide" vs "sem consumir") |
| State do Jev com sufixo "This step includes image attachments." | SIM | `buildState(userInput, step, hasImages)`; sufixo literal ao final do state; state nunca contém base64 (assertado no teste 10) |
| Pin em modelo sem visão com anexos ignorado | SIM | Guard `(!hasImages \|\| m.Vision)` no ramo do pin; pin em modelo COM visão continua vencendo (decisão 4 dos Riscos Conhecidos) |
| Fallback de baixa confiança respeita visão | SIM | Disponibilidade do default checada contra a lista **já filtrada** — default sem visão nunca é escolhido quando há imagens |
| `session.Event.Attachments []AttachmentMeta{name, size}` (`json:"attachments,omitempty"`) gravado no evento `user` | SIM | Struct e tag exatos; resolve REQ-005 no transcript |
| `WriteSnapshot` serializa parts de imagem como placeholder `{"type":"image_url","image_url":{"url":"[omitted]"}}` | SIM | `snapshotMessages`/`omitImageParts`; cópia defensiva não muta as mensagens vivas (o gateway continua recebendo o base64 real nos passos seguintes do turno); resolve a Ressalva 2 da task 3.0 |
| Text parts preservadas no snapshot | SIM | Só parts `image_url` viram placeholder; roundtrip de text parts coberto por teste |
| Base64 só ao gateway — nunca ao Jev, nunca ao transcript | SIM | Provado por testes automáticos: state do Jev sem "base64" (teste 10); JSONL inteiro sem "base64," (teste 13 + teste de session) |
| TUI: enter limpa pendentes, exceto no erro de roteamento, que os preserva | SIM | `sentAttachments` + `turnConsumed` (marcado no primeiro `EventRoute`); erro pré-consumo restaura os pendentes com chips e chrome — ver Ressalva 2 (generalização deliberada) |
| Ressalva 3 da task 3.0 (roteamento por visão) | SIM | Núcleo desta task |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 4.1 `RunWithAttachments` com `Run` como wrapper | COMPLETA | Assinatura com meta; wrapper preserva callers |
| 4.2 Filtrar candidatos por `Vision` em `decide`; sufixo no state do Jev | COMPLETA | Filtro pós-enriquecimento TPS; sufixo no state; provado por captura de criteria + state no mock Jev |
| 4.3 Erro amigável sem consumir a mensagem | COMPLETA | Pre-check antes do append; `messages` vazio e zero chamadas ao gateway assertados |
| 4.4 `AttachmentMeta` no `session.Event` gravado no evento `user` | COMPLETA | Evento `user` com `attachments: [{name, size}]` assertado no JSONL |
| 4.5 Placeholder no `WriteSnapshot` | COMPLETA | Formato exato assertado byte a byte; não-mutação da mensagem viva assertada |
| 4.6 Testes 10-13 da techspec | COMPLETA | 4 testes exigidos + 5 adicionais (pin, snapshot session ×2, preservação TUI ×2) |

## Testes
- Total de Testes: 160
- Passando: 160
- Falhando: 0
- Coverage: N/A (sem tool de coverage configurado; verificação por contagem: 151 pré-existentes + 9 novos, todos passando; `go test -race` limpo em `internal/agent`, `internal/session`, `internal/tui`)

Testes exigidos pela task (itens 10-13 da techspec):
- `TestVisionFilterRoutesOnlyVisionModels` — catálogo com 2 modelos vision (força consulta real ao Jev; com 1 único candidato o router short-circuita sem consultá-lo): criteria contém exatamente {glm-5.2, glm-5.3}, nenhum modelo sem visão oferecido, state contém "image attachments" e não contém "base64", request ao gateway no modelo escolhido pelo Jev.
- `TestNoVisionModelPreservesAttachments` — gateway expondo só glm-5.2 (sem visão): `EventError` "no vision-capable model available"; `ag.messages` vazio (turno não consumido); zero chamadas de chat ao gateway.
- `TestRequestContainsContentParts` — request capturado byte a byte: `"content":[{"type":"text",...},{"type":"image_url","image_url":{"url":"data:image/png;base64,...}}]`; decodificação via `UnmarshalJSON` confirma parts texto + imagem com data URI exato.
- `TestTranscriptHasNoBase64` — JSONL inteiro sem "base64,"; snapshot contém `"[omitted]"`; evento `user` com `attachments: [{name: shot.png, size: 7}]` e conteúdo de texto preservado.

Testes adicionais da camada (edge cases):
- `TestPinWithoutVisionIgnoredWithAttachments` — pin em deepseek-v4-flash (sem visão) ignorado com anexos (rota via Jev para glm-5.2); pin em glm-5.3 (com visão) vence com anexos (rota "pin").
- `TestWriteSnapshotOmitsImageParts` (session) — placeholder exato; sem "base64,"; `WriteSnapshot` não muta a mensagem viva.
- `TestWriteSnapshotKeepsTextParts` (session) — text parts sobrevivem ao snapshot/load sem alteração.
- `TestRoutingErrorPreservesAttachments` (TUI) — erro de roteamento restaura pendentes (chip, nome, chrome do viewport, busy=false, erro amigável no chat).
- `TestStreamErrorAfterRouteKeepsAttachmentsConsumed` (TUI) — erro pós-rota NÃO restaura pendentes (turno consumido); todos os eventos bombeados pela TUI (o helper inicial descartava o EventRoute e foi corrigido durante a task).

## Verificação de Segurança
- N/A para backend/API/endpoints — TUI/agent local. Privacidade (requisito não negociável do PRD): base64 existe apenas no request HTTP ao gateway — provado por testes automáticos de não-vazamento no state do Jev (teste 10) e no transcript inteiro (teste 13, teste de session). O snapshot sanitiza parts de imagem; as mensagens vivas em `a.messages` preservam o base64 apenas em memória para os passos seguintes do turno. Sem secrets, sem dados sensíveis em logs.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | loop/decide | Ressalva 1 (interpretação): a techspec diz "filtragem em `decide` ... vazio → emitError **sem consumir a mensagem**", mas `decide` roda após o append ao histórico — um erro vindo de `decide` consumiria a mensagem. O check de vazio foi colocado como pre-check em `loop` (antes do append); a filtragem em si permanece em `decide` conforme especificado | Manter: é a única forma de conciliar as duas cláusulas ("em decide" + "sem consumir"). Documentado aqui para rastreabilidade |
| Baixa | internal/tui/tui.go | EventError | Ressalva 2 (generalização deliberada): a techspec pede preservação dos pendentes "no erro de roteamento"; a implementação restaura em **qualquer** erro pré-consumo (gateway não configurado, falha ao listar modelos, erro de visão) — sinal de consumo é o primeiro `EventRoute` | Manter: o princípio subjacente é "turno não iniciado → nada consumido → restaurar"; o caso do erro de roteamento está coberto exatamente como especificado; os demais casos são estritamente mais amigáveis e não contradizem a spec |
| Baixa | internal/agent/agent.go | buildSummaryPrompt | Ressalva 3 (gap de feature, sem task dona): a techspec (Dependências Técnicas) documenta que, na compação, parts de imagem viram `[image attached: nome]` no prompt de resumo — não implementado (o nome do anexo não vive na `llm.Message`; e compação não está no escopo de nenhuma task 1.0–5.0). Sem violação de privacidade: o resumo usa só `m.Content` (texto), o base64 não vaza; a imagem apenas desaparece do resumo | Triar na task 5.0 / `kspec-pr-review`: ou registrar como limitação documentada, ou reabrir uma task para propagar metadados de anexo ao histórico |
| Baixa | internal/agent/agent.go | buildState | O sufixo "This step includes image attachments." fica ao final do state e pode ser truncado em inputs próximos de `maxStateChars` (4000) | Cosmético: o roteamento por visão não depende do state (a filtragem é por código); reposicionar o sufixo antes da instrução final, se desejado |

## Pontos Positivos
- Requisito não negociável de privacidade garantido por construção e provado por 3 testes automáticos de não-vazamento (state do Jev, transcript via agent, snapshot via session) — o base64 existe só no request HTTP.
- `WriteSnapshot` sanitiza uma cópia defensiva: o histórico vivo mantém o base64 em memória para os passos seguintes do turno (o gateway precisa da imagem a cada passo), enquanto o JSONL grava só o placeholder — exatamente a mitigação documentada nos Riscos Conhecidos.
- O fallback de baixa confiança checa o default contra a lista já filtrada: com anexos, um default sem visão nunca é escolhido — o bug latente de "default não-vision via fallback de confiança" foi fechado junto com a filtragem principal.
- Preservação de pendentes na TUI sem novo campo no `agent.Event`: o sinal de consumo é o primeiro `EventRoute` já existente — zero acoplamento novo entre agent e TUI; o caso pós-rota (não restaurar) tem teste próprio.
- Teste 10 usa catálogo com 2 modelos vision por necessidade real: com 1 único candidato o `JevRouter` short-circuita sem consultar o Jev — o teste documenta essa armadilha e valida a criteria real recebida.
- O teste de TUI `TestStreamErrorAfterRouteKeepsAttachmentsConsumed` pegou um defeito no próprio harness de teste (eventos descartados sem passar pela TUI) — corrigido com `pumpUntilAgentError`, que bombeia todos os eventos pela model, como o bubbletea faria.

## Recomendações
- Task 5.0: triar a Ressalva 3 (interação compação × anexos) — decidir entre limitação documentada ou task de follow-up.
- `kspec-qa` (E2E): os 3 cenários da techspec — `/image` + pergunta com resposta descrevendo a imagem; compatibilidade sem anexos; erro amigável com anexos preservados no fluxo real.
- Opcional: reposicionar o sufixo do state antes da instrução final para imunizá-lo ao truncamento (Ressalva 4).

## Checks Executados
- `go build ./...` — OK
- `go vet ./...` — OK
- `gofmt -l .` — vazio (nenhum arquivo a formatar)
- `go test ./... -count=1` — 160/160 passando, 0 falhas (inclui os 151 pré-existentes)
- `go test -race -count=1 ./internal/agent/ ./internal/session/ ./internal/tui/` — OK (sem data races)

## Conclusão
APROVADO COM RESSALVAS. A task 4.0 está completa conforme a definição e a techspec: `RunWithAttachments(text, parts, meta)` com a assinatura exata (Ressalva 1 da task 3.0 resolvida), `Run` como wrapper preservando todos os callers, filtragem de candidatos por `Vision` em `decide` com routers intocados, sufixo "This step includes image attachments." no state do Jev, erro amigável "no vision-capable model available" sem consumir o turno (mensagem fora do histórico, zero chamadas ao gateway), pin em modelo sem visão ignorado com anexos, fallback de baixa confiança respeitando visão, `attachments: [{name, size}]` no evento `user` do transcript e placeholder `"[omitted]"` no snapshot (Ressalvas 2 e 3 da task 3.0 resolvidas — requisito não negociável de privacidade com testes automáticos de não-vazamento). A preservação de pendentes na TUI no erro de roteamento atende a história de usuário do REQ-004. Os 4 testes exigidos (10-13) provam os critérios de sucesso, com 5 testes adicionais cobrindo pin, placeholder de session e os dois ramos da preservação na TUI. As ressalvas são interpretações documentadas (posição do check de vazio; generalização da restauração) e um gap de feature sem task dona (interação compação × anexos, sem violação de privacidade) — nenhuma bloqueia o fechamento; a Ressalva 3 deve ser triada na task 5.0. Todos os checks passam com os 151 testes pré-existentes intactos.
