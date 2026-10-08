# Relatório de Code Review - Anexos de imagem no prompt (Task 5.0: Verificação final integrada e checagem de critérios de aceite)

## Resumo
- Data: 2026-10-04
- Branch: working tree (baseline a2fa08f; mudanças 001–007 e 008/1.0–4.0 não commitadas preservadas — os números de diff abaixo cobrem o working tree inteiro, que inclui as features anteriores nos mesmos arquivos)
- Status: APROVADO COM RESSALVAS
- Arquivos da feature 008: 11 (`internal/llm/llm.go`, `internal/llm/llm_test.go`, `internal/catalog/catalog.go`, `internal/catalog/models.yaml`, `internal/catalog/catalog_test.go`, `internal/tui/tui.go`, `internal/tui/tui_test.go`, `internal/agent/agent.go`, `internal/agent/agent_test.go`, `internal/session/session.go`, `internal/session/session_test.go`)
- Diff do working tree (todas as features): 16 arquivos modificados, +5878/−178, mais 10 arquivos `.go` novos
- Task 5.0 não escreve código novo — verificação apenas; nenhuma linha de código alterada por esta review

## Subtarefa 5.1 — Verificação encadeada

`go build ./... && go vet ./... && gofmt -l . && go test ./... -count=1` executado em cadeia única: **tudo verde**.

- `go build ./...` — OK
- `go vet ./...` — OK
- `gofmt -l .` — saída vazia (nenhum arquivo a formatar)
- `go test ./... -count=1` — 160/160 testes passando, 0 falhas, 8 packages com testes (`kterminal`, `agent`, `catalog`, `llm`, `session`, `telemetry`, `tools`, `tui`)

Extra: `go test -race -count=1 ./internal/agent/ ./internal/session/ ./internal/tui/` — limpo em 30+ execuções (ver Problemas Encontrados, item informativo sobre 1 crash transitório não reproduzível).

## Subtarefa 5.2 — Checklist de critérios de aceite do PRD (evidência por item)

| # | Critério de aceite (PRD) | Evidência | Status |
|---|--------------------------|-----------|--------|
| 1 | REQ-001: Mensagem com imagem chega ao gateway como array de content parts | `TestRequestContainsContentParts` (agent_test.go:2387) — decodifica o request real capturado: `messages[0].content` é array `[{type:text},{type:image_url, image_url:{url:"data:image/png;base64,..."}}]`; reforçado por `TestSendWithAttachmentsBuildsParts` (TUI→agent→gateway) e `TestMessageMarshalContentParts` (unit) | OK |
| 2 | REQ-001: Sem anexos, o request é idêntico ao atual | `TestMessageMarshalStringContentGolden` (llm_test.go:51) — golden byte a byte em 5 casos (user, content vazio, escapes, assistant+tool_calls, tool result); `chatRequest` inalterado; TUI chama `Run` (wrapper) quando não há pendentes | OK |
| 3 | REQ-002: Catálogo expõe visão por modelo, vinda do YAML | `TestVisionParsedFromYAML` (catalog_test.go:52) — `glm-5.3` → `Vision: true`, demais → false; `models.yaml` com `vision` em todos os 7 modelos; `Model.Vision bool \`yaml:"vision"\`` | OK |
| 4 | REQ-003: Arquivo inexistente/extensão inválida → erro claro, nada anexado; >5MB → rejeitado | `TestImageRejectsInvalid` (tui_test.go:1715) — 4 rejeições (inexistente, `.txt`, 6MB, subdiretório): erro inline em `colError`, 0 pendentes, prompt não abortado (`stateChat`), sem chip; whitelist `imageMimes` png/jpg/jpeg/gif/webp; `maxAttachmentBytes` = 5MB validado via `os.Stat` antes da leitura | OK |
| 5 | REQ-003: Chips visíveis acima do prompt; `/unimage` remove o anexo correto | `TestImageCommandAttaches` (chip entre chat e prompt, ANSI `colSecondary` #5c9cf5, extensão maiúscula `.PNG` → `image/png`, chrome +1 row) + `TestUnimageRemovesAttachment` (remove índice 1 de 2, `two.png` permanece; bordas 0/3/abc/sem-args não removem nada) | OK |
| 6 | REQ-004: Com anexo, o Jev só recebe modelos `vision: true` | `TestVisionFilterRoutesOnlyVisionModels` (agent_test.go:2325) — criteria capturada no mock Jev contém exatamente {glm-5.2, glm-5.3} (os 2 únicos vision do catálogo de teste); state contém "image attachments" e não contém "base64"; request vai ao modelo escolhido pelo Jev | OK |
| 7 | REQ-004: Nenhum modelo de visão → erro amigável, anexos preservados | `TestNoVisionModelPreservesAttachments` (agent) — `EventError` "no vision-capable model available", `messages` vazio (turno não consumido), 0 chamadas ao gateway; `TestRoutingErrorPreservesAttachments` (TUI) — pendentes restaurados com chips após o erro | OK |
| 8 | REQ-005: JSONL contém metadados dos anexos, sem base64 | `TestTranscriptHasNoBase64` (agent_test.go:2425) — arquivo inteiro sem `"base64,"`; snapshot com `"[omitted]"`; evento `user` com `attachments: [{name: shot.png, size: 7}]` e texto preservado; `TestWriteSnapshotOmitsImageParts` + `TestWriteSnapshotKeepsTextParts` (session) | OK |
| 9 | REQ-003 (comando): `/image` sem args lista pendentes | `TestImageWithoutArgsLists` — lista vazia e lista com "1. pic.jpg (7B)"; mime jpg → `image/jpeg` | OK |

Experiência do usuário (PRD): fluxo `/image` → chip → pergunta → enter sem passo extra (`TestSendWithAttachmentsBuildsParts`); bloco do usuário no chat mostra texto + chips (`userBlock`); erros de anexo inline sem abortar o prompt (`TestImageRejectsInvalid`).

**Nenhum requisito do PRD deixado de lado.**

## Subtarefa 5.3 — Inspeção do diff contra os padrões do projeto

- **Sem comentários no código**: `git diff` filtrado por linhas `+` com `//`/`/*` — zero ocorrências (única diretiva `//go:embed` em catalog.go é pré-existente e é diretiva de compilador, não comentário).
- **Base64 nunca no transcript nem no state do Jev**: grep de `base64,` em código de produção — única ocorrência em `tui.go:941` (construção do data URI em memória, caminho intencional rumo ao gateway). `session.go` não contém base64 (snapshot sanitiza parts de imagem para `[omitted]` via cópia defensiva); `agent.go` — `buildState` nunca toca nas parts e `buildSummaryPrompt` usa só `m.Content`. Três testes automáticos de não-vazamento (state do Jev, transcript, snapshot) provam o requisito não negociável.
- **Eventos via canal**: erro de visão via `emitError` → canal `Events` + evento `error` no transcript — padrão existente.
- **Receivers por valor na TUI**: `handleAgentEvent`, `View`, handlers de update — todos `func (m Model)`.
- **Paleta**: chips em `chipStyle` com `colSecondary` (assertado por ANSI nos testes).
- **Fora de escopo do PRD — confirmado NÃO implementado**: colar imagem do clipboard (`internal/clipboard` é pré-existente e só copia texto — `clipboard.Copy`); captura de screenshot (nenhum); PDF/vídeo/áudio (whitelist só png/jpg/jpeg/gif/webp); OCR (nenhum); redimensionamento (leitura bruta + encode, sem processamento); visão em mensagens do agente (`ContentParts` só existe na mensagem de usuário — agent.go:152; mensagens assistant/tool nunca carregam parts).

## Subtarefa 5.4 — Prontidão para o `kspec-qa` (cenários E2E da techspec)

1. **Anexo + pergunta**: `/image screenshot.png` → chip aparece → pergunta sobre a imagem → enter → linha de rota indica modelo com visão (glm-5.3) → resposta descreve a imagem.
2. **Compatibilidade sem anexos**: prompt só-texto → comportamento e request idênticos ao pré-feature (roteamento Jev normal, sem regressão).
3. **Erro amigável sem modelo de visão**: gateway expondo apenas modelos sem visão → erro "no vision-capable model available" inline → anexos preservados como chips → `/unimage` remove → reenvio funciona sem duplicar mensagens.

Regressão recomendada no mesmo passe: `/image` sem args (lista), `/unimage` com índice inválido (erro claro), envio de segunda mensagem após turno com imagem (histórico íntegro no chat).

## Triagem da Ressalva 3 da review 4.0 (compação × anexos)

**Decisão: pendência registrada para o `kspec-pr-review` — não implementar na task 5.0.**

A techspec 008 (Dependências Técnicas) documenta: na compação, a mensagem com parts é preservada na cauda (se entre as últimas 4) **ou** vai ao resumo como texto com parts de imagem virando `[image attached: nome]`. Estado atual:

- Preservação na cauda com parts: **implementada** (o tail mantém as mensagens intactas; o base64 segue ao gateway nos passos seguintes do turno).
- Menção `[image attached: nome]` no prompt de resumo: **não implementada** — `buildSummaryPrompt` usa só `m.Content`; a imagem some silenciosamente do resumo.

Razões da triagem como pendência (e não implementação aqui):

1. A task 5.0 é explícita: "Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0)" — e nenhuma task 1.0–4.0 tem a interação compação no escopo (grep nos 4 arquivos de task: zero menções a compação/resumo/003). Gap documentado sem task dona.
2. O PRD não exige: REQ-001 a REQ-005 estão todos atendidos; o requisito não negociável de privacidade está garantido (o resumo usa só texto — o base64 não vaza em nenhum caminho).
3. Implementar exigiria propagar o nome do anexo até `llm.Message` (novo campo ou rastreamento paralelo no agent) — mudança cross-cutting em llm/agent/TUI, incompatível com uma task P de verificação e com o mandato anti-scope-creep.
4. Impacto atual: degradação menor de contexto em sessões longas (o resumo perde a memória de que houve imagem); sem violação de privacidade; sem quebra funcional (o roteamento por visão é por turno, baseado na user message corrente — `decide(ctx, userMessage, step)`).

**Encaminhamento**: o `kspec-pr-review` deve decidir entre (a) aceitar como limitação documentada, ou (b) abrir task de follow-up para propagar metadados de anexo ao histórico e renderizar `[image attached: nome]` no prompt de resumo.

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| code-standards.md | OK | Rule vazia no repo; padrões do projeto seguidos: zero comentários no diff, nomenclatura consistente, gofmt limpo |
| architecture-ddd.md | N/A | Brownfield Go com packages `internal/`, conforme techspec |
| tests.md (Vitest) / demais rules de stack | N/A | Stack Go — `testing` stdlib + `httptest`, AAA, temp dir, golden byte a byte |
| Padrões do projeto (eventos via canal, receivers por valor, paleta) | OK | Verificados por inspeção na subtarefa 5.3 |
| Sem comentários no código | OK | Zero comentários adicionados em fontes e testes |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `ContentPart`/`ImageURL` + `MarshalJSON` condicional (string sem parts, array com parts) | SIM | Golden trava o formato string; roundtrip cobre snapshot da 004 |
| `vision` no YAML + `Model.Vision` | SIM | glm-5.3 true; demais false (default honesto) |
| `/image`/`/unimage`, validação (whitelist, 5MB, existência), chips `colSecondary` | SIM | Erros inline sem abortar o prompt |
| `RunWithAttachments(text, parts, meta)` com `Run` wrapper | SIM | Callers existentes inalterados |
| Filtragem por `Vision` no agent (routers intocados); state com sufixo de anexos | SIM | Fallback de baixa confiança respeita a lista filtrada; pin sem visão ignorado com anexos |
| Erro amigável sem consumo do turno | SIM | Pre-check antes do append; preservação na TUI via sinal `EventRoute` |
| `AttachmentMeta` no evento `user`; snapshot com placeholder `[omitted]` | SIM | Testes de session e agent |
| Base64 só ao gateway | SIM | 3 testes automáticos de não-vazamento + inspeção |
| Interação techspec 003: `[image attached: nome]` no prompt de resumo | PARCIAL | Cauda preservada com parts; menção no resumo não implementada — triada nesta task como pendência para o `kspec-pr-review` (sem task dona; sem impacto no PRD; sem vazamento) |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 1.0 Content parts no `llm.Message` | COMPLETA | Review 1.0: APROVADO |
| 2.0 Capacidade de visão no catálogo | COMPLETA | Review 2.0: APROVADO |
| 3.0 Comandos `/image`/`/unimage` e chips | COMPLETA | Review 3.0: APROVADO COM RESSALVAS (ressalvas resolvidas na 4.0) |
| 4.0 `RunWithAttachments`, roteamento, transcript | COMPLETA | Review 4.0: APROVADO COM RESSALVAS (Ressalva 3 triada aqui; 1, 2 e 4 documentadas/mantidas) |
| 5.1 Checks encadeados | COMPLETA | Cadeia única, tudo verde |
| 5.2 Checklist de aceite com evidência | COMPLETA | 9/9 itens com teste que o cobre (tabela acima) |
| 5.3 Inspeção de padrões do diff | COMPLETA | Sem comentários; sem `base64,` em transcript/state; fora de escopo não implementado |
| 5.4 Cenários E2E para o QA | COMPLETA | 3 cenários da techspec + regressões listados acima |

## Testes
- Total de Testes: 160
- Passando: 160
- Falhando: 0
- Coverage: N/A (sem tool de coverage configurado; verificação por contagem e por assertes de comportamento real — os testes decodificam requests capturados, verificam ANSI/posição de chips, leem o JSONL gerado)
- E2E: nenhum novo nesta task (deferido ao `kspec-qa` conforme a task); integração coberta pelos testes de agent com mocks (TUI→agent→gateway→transcript)

## Verificação de Segurança
- N/A para backend/API/endpoints — TUI/agent local. Privacidade (requisito não negociável): base64 existe apenas no request HTTP ao gateway — provado por 3 testes automáticos de não-vazamento (state do Jev, transcript, snapshot) e por inspeção (única ocorrência de `base64,` em produção é a construção do data URI na TUI). Whitelist de extensões (não blacklist); limite de 5MB validado via `os.Stat` antes da leitura; sem secrets; sem dados sensíveis em logs.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | buildSummaryPrompt | Pendência (triada nesta task): menção `[image attached: nome]` no prompt de resumo da compação não implementada — gap documentado na techspec sem task dona; sem violação de privacidade; sem impacto nos critérios de aceite do PRD | `kspec-pr-review`: aceitar como limitação documentada ou abrir task de follow-up (propagar metadados de anexo ao histórico) |
| Baixa | internal/agent/agent.go | buildState | Sufixo "This step includes image attachments." ao final do state pode ser truncado em inputs próximos de `maxStateChars` (herdado da review 4.0, Ressalva 4) | Cosmético: o roteamento não depende do state; reposicionar antes da instrução final, se desejado |
| Informativo | teste (ambiente) | — | 1 crash transitório do binário de teste sob `-race` (SIGSEGV "fault", sem report de DATA RACE) na primeira execução; não reproduzido em 30+ execuções subsequentes (`-count=1` ×10, `-count=5`, `-count=10` ×2), incluindo a cadeia obrigatória repetida | Nenhuma ação: sem data race reportado; provável pressão de memória do host; monitorar se recorrer |

## Pontos Positivos
- Checklist de aceite do PRD 9/9 com evidência automática — cada critério tem teste que decodifica o artefato real (request HTTP capturado, JSONL gerado, criteria/state capturados no mock Jev, ANSI/posição de chips na View).
- Requisito não negociável de privacidade garantido por construção e triplo teste de não-vazamento — o base64 existe só no request HTTP; o snapshot sanitiza cópia defensiva sem mutar o histórico vivo.
- Compatibilidade total travada por golden byte a byte: regressão de contrato no formato string é impossível de passar despercebida.
- Verificação de fora-de-escopo confirmada por inspeção: nenhuma funcionalidade além do PRD foi implementada (clipboard é texto pré-existente; whitelist fecha PDF/vídeo/áudio; parts só em mensagens de usuário).
- A feature fecha pronta para os dois próximos passos do fluxo kspec: cenários E2E listados para o `kspec-qa` e pendência única, triada e documentada, para o `kspec-pr-review`.

## Recomendações
- `kspec-qa`: executar os 3 cenários E2E da techspec (anexo+pergunta com resposta descrevendo a imagem; compatibilidade sem anexos; erro amigável com anexos preservados) + as regressões listadas na subtarefa 5.4.
- `kspec-pr-review`: triar a pendência da compação (limitação documentada vs. task de follow-up).
- Opcional: reposicionar o sufixo do state antes da instrução final (imunidade ao truncamento).

## Checks Executados
- `go build ./...` — OK
- `go vet ./...` — OK
- `gofmt -l .` — vazio (nenhum arquivo a formatar)
- `go test ./... -count=1` — 160/160 passando, 0 falhas
- `go test -race -count=1 ./internal/agent/ ./internal/session/ ./internal/tui/` — OK (30+ execuções limpas; 1 crash transitório não reproduzível documentado acima)

## Conclusão
APROVADO COM RESSALVAS. A verificação final integrada da feature 008 confirma: cadeia obrigatória completa verde em execução única (build, vet, gofmt vazio, 160/160 testes); os 9 critérios de aceite do PRD percorridos um a um com teste de evidência cada (REQ-001 golden + parts no request; REQ-002 visão do YAML; REQ-003 validações/chips//unimage; REQ-004 filtro de visão com Jev decisor e erro amigável sem consumo do turno; REQ-005 transcript com metadados sem base64); padrões do projeto confirmados por inspeção do diff (zero comentários, eventos via canal, receivers por valor, base64 só ao gateway); nenhum requisito do PRD deixado de lado e nenhum item fora de escopo implementado. A Ressalva 3 da review 4.0 (interação compação × anexos) foi triada conforme o mandato da task: gap documentado na techspec sem task dona, sem impacto nos critérios de aceite do PRD e sem violação de privacidade — registrado como pendência para o `kspec-pr-review` decidir entre limitação documentada ou task de follow-up. As demais ressalvas são cosméticas/informativas e não bloqueiam. A funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.
