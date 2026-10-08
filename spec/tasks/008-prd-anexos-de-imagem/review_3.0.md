# Relatório de Code Review - Anexos de imagem no prompt (Task 3.0: Comandos `/image`/`/unimage` e chips de anexos na TUI)

## Resumo
- Data: 2026-10-04
- Branch: working tree (baseline a2fa08f; mudanças 001–007 e 008/1.0+2.0 não commitadas preservadas — a camada desta review é apenas a task 3.0)
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 4 (`internal/agent/agent.go`, `internal/tui/theme.go`, `internal/tui/tui.go`, `internal/tui/tui_test.go`)
- Linhas Adicionadas: ~455 (camada 3.0: ~168 em `tui.go`, 282 em `tui_test.go`, 4 em `theme.go`, ~9 em `agent.go`)
- Linhas Removidas: ~10 (substituições: `userBoxStyle.Render(value)` → `userBlock`, `m.agent.Run` → ramo condicional, corpo de `viewportHeight`, assinatura de `loop`)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| code-standards.md | OK | Rule vazia no repo; padrões do projeto seguidos: zero comentários adicionados (único `//` no arquivo é literal de string pré-existente), nomenclatura consistente, gofmt limpo |
| architecture-ddd.md | N/A | Brownfield Go com packages `internal/`, conforme techspec |
| tests.md (Vitest) / demais rules de stack | N/A | Stack Go — `testing` stdlib + `httptest`, padrão do repositório |
| Padrões do projeto (receivers, paleta, eventos) | OK | Mutadores da TUI com receiver ponteiro (mesmo padrão de `refreshContent`/`fitViewportHeight`); handlers do caminho Update por valor (padrão existente); chips em `colSecondary` via `chipStyle`; erros de anexo inline via `errorBoxStyle` (borda `colError`), sem `EventError` |
| Sem comentários no código | OK | Verificado por grep — nenhum comentário na camada nova |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `attachment{name, size, dataURI}` + `pendingAttachments` no Model | SIM | Struct e campo exatamente como a techspec |
| `/image <path>`: extensão whitelist png/jpg/jpeg/gif/webp → mime | SIM | Map `imageMimes` com mime correto por extensão (jpg/jpeg → image/jpeg); case-insensitive via `strings.ToLower` |
| `os.Stat` ≤ 5MB antes da leitura | SIM | `maxAttachmentBytes = 5*1024*1024`; tamanho validado antes de `os.ReadFile`; diretório rejeitado |
| `base64.StdEncoding` → `data:<mime>;base64,<...>` | SIM | Data URI construída exatamente no formato especificado |
| Erros inline em `colError`, sem abortar o prompt, nada anexado | SIM | `errorBoxStyle` (border `colError`) no chat; estado permanece `chat`; prompt utilizável; pendentes intocados |
| `/image` sem args → lista pendentes no chat | SIM | Lista numerada 1-based com nome e tamanho; mensagem clara quando vazio |
| `/unimage <n>` remoção 1-based | SIM | Índice validado (não-numérico, 0, out-of-range → erro inline); confirmação "removed `<nome>`" |
| Chips `🖼 nome ×` em `colSecondary` acima do prompt | SIM | `renderAttachmentChips` + `chipStyle` (colSecondary); linha entre popup de menções e o prompt box |
| Bloco do usuário no chat: texto + chips dos anexos enviados | SIM | `userBlock` renderiza texto + chips dentro do user box |
| Enter com anexos → parts na ordem texto + imagens, pendentes limpos | SIM | `attachmentParts` (text primeiro, depois image_url na ordem dos pendentes); pendentes limpos pós-envio; exceção do erro de roteamento é a task 4.0 |
| Envio pelo método do agent com parts | SIM | `RunWithAttachments(text, parts)` — stub/wiring fino conforme a task; `Run` vira wrapper com parts nil (callers inalterados); `loop` recebe `llm.Message` |
| Assinatura final `RunWithAttachments(text, parts, meta []session.AttachmentMeta)` | PARCIAL (por design) | O parâmetro `meta` exige `session.AttachmentMeta`, que é criado na task 4.0 (REQ-005). A task 3.0 pede explicitamente "stub/wiring fino; a semântica de roteamento é a task 4.0". Ver Ressalva 1 |
| Filtragem por visão, state do Jev com anexos, erro amigável preservando pendentes | NÃO (fora do escopo) | Task 4.0, conforme techspec e tasks.md |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 3.1 `pendingAttachments` + `/image <path>` com validações e base64 | COMPLETA | Existência, diretório, extensão, 5MB, leitura e codificação; ordem de validação segue a task (existência → extensão → tamanho), todas antes da leitura |
| 3.2 `/image` sem args (lista) + `/unimage <n>` (1-based) | COMPLETA | Inclui tratamento de args ausentes/inválidos |
| 3.3 Chips acima do prompt e no bloco do usuário enviado | COMPLETA | ColSecondary; linha extra de chrome refletida no `viewportHeight` (viewport encolhe 1 linha com chips visíveis) |
| 3.4 Envio com anexos ligado ao método do agent (parts texto + imagens) | COMPLETA | `RunWithAttachments` stub no agent; TUI constrói parts e limpa pendentes |
| 3.5 Testes 5-9 da techspec | COMPLETA | 5 testes novos, todos passando |

## Testes
- Total de Testes: 151
- Passando: 151
- Falhando: 0
- Coverage: N/A (sem tool de coverage configurado; verificação por contagem: 146 pré-existentes + 5 novos, todos passando; `go test -race` limpo em `internal/tui` e `internal/agent`)

Testes exigidos pela task (itens 5-9 da techspec):
- `TestImageCommandAttaches` — anexo válido com data URI exato (base64 decodificável), chip em colSecondary com ANSI truecolor verificado, posição do chip entre chat e prompt, extensão maiúscula `.PNG` → image/png, chrome do viewport com chips.
- `TestImageRejectsInvalid` — inexistente / `.txt` / 6MB / diretório → 4 erros inline em colError, zero anexos, estado chat preservado, sem chips, mensagens específicas assertionadas.
- `TestUnimageRemovesAttachment` — 2 anexos + `/unimage 1` → `two.png` permanece com chip, chip de `one.png` removido, confirmação no chat; `/unimage 0/3/abc` e sem args → erro inline e nada removido.
- `TestSendWithAttachmentsBuildsParts` — captura do request no gateway: content é array byte a byte (`[{"type":"text",...},{"type":"image_url",...}]`), parts decodificadas com texto + image_url (data URI exato), chat com texto + chips (ANSI colSecondary no user block), pendentes limpos, chrome restaurado.
- `TestImageWithoutArgsLists` — lista vazia ("no pending attachments") e lista com entrada numerada `1. pic.jpg (7B)`; mapeamento jpg → image/jpeg.

Edge cases cobertos além do caminho feliz: extensão maiúscula, diretório, índices 0/out-of-range/não-numérico, args ausentes, lista vazia, altura do viewport com/sem chips, formato byte a byte do array de parts.

## Verificação de Segurança
- N/A para backend/API/endpoints — TUI local. Validações implementadas: whitelist de extensões (não blacklist), limite de 5MB via `os.Stat` antes da leitura (protege payload e memória), índice 1-born validado, paths tratados como dados (sem execução de conteúdo — apenas transporte em data URI). Sem secrets, sem dados sensíveis em logs: o evento `user` do transcript grava apenas o texto (sem base64 — ver Ressalva 2 para o snapshot interino).

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Média | internal/agent/agent.go | RunWithAttachments/loop | Ressalva 2: com o stub, `WriteSnapshot(a.messages)` serializa a mensagem user com `ContentParts` — o evento `snapshot` do JSONL contém o base64 até a mitigação da task 4.0 | Task 4.0 DEVE implementar o placeholder `{"type":"image_url","image_url":{"url":"[omitted]"}}` em `WriteSnapshot` (mitigação já documentada na techspec, seção Riscos Conhecidos) |
| Baixa | internal/agent/agent.go | RunWithAttachments | Ressalva 1: assinatura do stub sem o parâmetro `meta []session.AttachmentMeta` da techspec | Task 4.0 estende a assinatura ao criar `session.AttachmentMeta` (REQ-005) e ajusta o único call site na TUI |
| Baixa | internal/tui/tui.go | attachImage | Ressalva 3: ordem de validação existência→extensão→tamanho (a techspec lista extensão antes de `os.Stat`; a task lista "existência, extensão e tamanho") | Mantida a ordem da task (erro mais claro para arquivo inexistente); todas as validações ocorrem antes da leitura — requisito de segurança atendido. Registrar como decisão |
| Baixa | internal/tui/tui.go | handleCommand /help | Ressalva 4: linha do `/help` adicionada além do texto literal da task | Adição mínima justificada: `/help` é o mecanismo de descoberta de comandos do projeto; sem ela os comandos novos seriam indescobríveis. Removível sem impacto se o time preferir estritamente o escopo |

## Pontos Positivos
- Envio com anexos end-to-end comprovado por captura de request: o teste valida o formato array byte a byte (contrato OpenAI-compatible da task 1.0 exercitado no fluxo real TUI→agent→gateway) e a ordem texto + imagens.
- `Run` como wrapper de `RunWithAttachments` preserva todos os 40+ callers de teste e produção sem alteração — exatamente a decisão 1 de compatibilidade da techspec aplicada ao agent.
- Chrome do viewport dinâmico: a linha de chips encolhe o viewport em 1 linha (sem overflow do hint bar), com `fitViewportHeight` chamado em anexar/remover/enviar/resize — coberto por 3 asserções de altura nos testes.
- Validações todas antes da leitura do arquivo; arquivo de 6MB do teste nunca é lido para memória pelo app.
- Reutilização sem duplicação: `renderAttachmentChips` serve ao prompt e ao bloco do usuário; `attachmentError` centraliza o erro inline.
- Testes seguem o padrão do repositório (temp dir, `forceTrueColor` para asserções ANSI, `captureGateway`, mensagens específicas assertionadas — não apenas "executou sem erro").

## Recomendações
- Task 4.0: estender `RunWithAttachments` com `meta []session.AttachmentMeta`, filtrar candidatos por `Vision`, sufixo no state do Jev, erro amigável sem consumo da mensagem (preservando pendentes na TUI — exigirá sinalização do agent para a TUI) e o placeholder de snapshot da Ressalva 2. Os testes 10-13 da techspec cobrem esses pontos.
- Considerar (task 4.0 ou QA): `/clear` não limpa pendentes — comportamento atual é defensável (pendentes são pré-envio), mas vale decisão explícita.

## Checks Executados
- `go build ./...` — OK
- `go vet ./...` — OK
- `gofmt -l .` — vazio (nenhum arquivo a formatar)
- `go test ./... -count=1` — 151/151 passando, 0 falhas (inclui os 146 pré-existentes)
- `go test -race -count=1 ./internal/tui/ ./internal/agent/` — OK (sem data races)

## Conclusão
APROVADO COM RESSALVAS. A task 3.0 está completa conforme a definição e a techspec: comandos `/image`/`/unimage` com todas as validações (existência, whitelist de extensões, 5MB via `os.Stat` antes da leitura, base64 data URI), erros inline em `colError` sem abortar o prompt, lista de pendentes, remoção 1-born, chips `🖼 nome ×` em `colSecondary` acima do prompt e no bloco do usuário, e o envio construindo parts na ordem texto + imagens entregues ao método novo do agent (stub fino, `Run` preservado como wrapper). Os 5 testes exigidos provam os critérios de sucesso, incluindo o contrato de parts capturado no gateway. As ressalvas são limites de escopo intencionais do sequenciamento (assinatura sem `meta` e mitigação de snapshot pertencem à task 4.0, que já as tem no título "transcript sem base64") e devem ser endereçadas lá — a Ressalva 2 é obrigatória para o requisito não negociável de privacidade do PRD. Todos os checks passam com os 146 testes pré-existentes intactos.
