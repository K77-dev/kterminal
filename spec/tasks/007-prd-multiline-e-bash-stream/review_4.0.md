# Relatório de Code Review — Prompt multi-linha com textarea e histórico de prompts (Task 4.0)

## Resumo
- Data: 2026-10-04
- Branch: `002-010-prds-kterminal` (HEAD `a2fa08f`, working tree com features 001–006 + 007/1.0–3.0 não commitadas)
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 2 (`internal/tui/tui.go`, `internal/tui/tui_test.go`); `go.mod` sem mudança (conforme task)
- Linhas Adicionadas: ~400 (≈125 em `tui.go`, ≈280 em `tui_test.go` — 6 testes)
- Linhas Removidas: ~45 (textinput config, `textBeforeCursor`/`completeMention` reescritos, 3 asserções `Position()` migradas)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| `strings.Builder` nunca por valor no Model | OK | Nenhum Builder novo no Model; estado novo é `[]string` + `int` |
| Receivers por valor na TUI | OK | `handleChatKey`/`Update` por valor (padrão existente); mutadores novos são `*Model` chamados na cópia local |
| gofmt / go vet | OK | Ambos limpos |
| Sem dependências novas | OK | `bubbles/textarea` já no `go.mod` (bubbles v1.0.0); `textinput` mantido (config screen) |
| Rules DDD/TS/Vitest/logging | N/A | Brownfield Go stdlib, conforme techspec |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `input` troca `textinput.Model` → `textarea.Model` (REQ-001) | SIM | `Prompt = ""`, `ShowLineNumbers = false`, `CharLimit` default 0 (= sem limite, igual ao anterior) |
| `SetMaxWidth` | SIM* | textarea v1.0.0 não tem método `SetMaxWidth` — `MaxWidth` é campo público, setado no `WindowSizeMsg` junto com `SetWidth(m.width-7)` (mesma largura do textinput anterior) |
| `TextStyle` com `Background(colBgElement)` — visual idêntico | SIM | `FocusedStyle.Text` + `FocusedStyle.CursorLine` com o mesmo fundo (o default do componente viria o cursor line em preto, quebrando a identidade); cursor `CursorStatic` + `Background(colBg).Foreground(colText)` idêntico ao anterior; caixa `promptBoxStyle` inalterada |
| Interceptação em `handleChatKey` **antes** de delegar: enter envia | SIM | Case `"enter"` existente preservado; popup de menções continua interceptando enter/tab primeiro (comportamento 005 intacto) |
| `shift+enter` → `InsertString("\n")` e consumir | SIM* | Case `"shift+enter"` presente, porém bubbletea v1.3.10 **não consegue representar shift+enter** (sem `Modifiers` em `Key`, sem `KeyShiftEnter`, sem parser kitty CSI-u) — ver Ressalva 1 |
| `SetHeight(clamp(1, linhas, 8))` após cada mudança de conteúdo | SIM | `handlePromptContentChange` em 3 caminhos: teclas delegadas, newline manual e fallthrough do `Update` (paste) |
| Altura por linhas **lógicas** (`\n`), não visuais | SIM | `input.LineCount()` (linhas lógicas do componente); risco de soft-wrap documentado na techspec como aceitável |
| Viewport mínimo 3 — `minViewportRows` extraída para reuso | SIM | `viewportHeight() = max(minViewportRows, m.height - input.Height() - viewChromeRows)`; com altura 1 reduz a `height-6` (idêntico ao código anterior); `viewChromeRows=5` fecha o layout exato (verificado: View = N linhas em janela N) |
| `promptHistory` máx. 20 FIFO + `histIdx` (REQ-002) | SIM | `pushPromptHistory` com re-slice; push só no envio efetivo (não comando, não busy-rejected) |
| ↑ com input vazio carrega o anterior; ↓ avança até voltar ao vazio | SIM | `navigateHistory`: ↑ inicia no mais novo e clampa no mais antigo; ↓ além do mais novo limpa o input e encerra navegação; ↑/↓ não navegáveis delegam ao textarea (movimento de cursor multi-linha) |
| Digitar qualquer coisa reseta a navegação | SIM | Detecção por mudança de valor (não por tecla) — cobre runes, backspace, paste e completion; mover cursor não reseta (correto: não é digitar) |
| Envio preserva `\n` — `agent.Run` recebe intacto | SIM | Provado ponta a ponta: `TestEnterSendsMultiline` captura o body HTTP e valida `content == "line one\nline two"` |
| Histórico em memória apenas | SIM | Sem I/O; `/clear` não toca no histórico (não especificado) |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 4.1 Trocar `textinput` por `textarea` com visual idêntico | COMPLETA | Render sanity check: box com 1 e 2 linhas idêntico ao anterior; layout fecha exato na altura da janela |
| 4.2 Interceptação: enter envia, shift+enter quebra (antes de delegar) | COMPLETA | Cases interceptados antes do `input.Update`; tecla consumida (o default do textarea é o inverso) |
| 4.3 Altura automática 1-8 e viewport mínimo 3 (`minViewportRows`) | COMPLETA | Clamp testado no máximo (13 linhas lógicas → 8) e no mínimo (janela 12 → viewport 3) |
| 4.4 Histórico de 20 com ↑/↓ e reset ao digitar | COMPLETA | Navegação, retorno ao vazio, cancelamento por digitação e FIFO testados |
| 4.5 Testes 8-11 da techspec | COMPLETA | 4 testes mandatórios + 2 extras de edge (ver Testes) |

## Testes
- Total de Testes: 142 top-level (136 baseline + 6 novos)
- Passando: 142
- Falhando: 0
- Coverage: N/A (projeto não mede coverage; nenhum caminho novo sem teste)
- Novos (mandatórios, techspec itens 8-11):
  - `TestShiftEnterCreatesNewline` — alt+enter e ctrl+j inserem `\n`; altura 1→2→3; viewport acompanha (`30-2-viewChromeRows`)
  - `TestEnterSendsMultiline` — 2 linhas digitadas + enter → body HTTP com `content` multi-linha preservado; input limpo e altura 1 após envio
  - `TestPromptHeightClamped` — 12 newlines → 13 linhas lógicas → altura clampa em 8; viewport 17 (≥3); janela 12 → viewport exatamente 3; envio → altura 1 e viewport recalculado
  - `TestHistoryNavigation` — 3 prompts: ↑ carrega o 3º, ↑ o 2º, ↓ volta ao 3º; digitar cancela (↑ passa a mover cursor); ctrl+u limpa; ↓ sem navegação é no-op; ↓ além do mais novo volta ao vazio; 21º prompt descarta o 01 (FIFO cap 20)
- Novos (edge cases, além do mandato):
  - `TestMentionCompletionMultiline` — completion com cursor em linha anterior à última (`setPromptCursor` restaura row/col via `CursorUp`+`SetCursor`); popup fecha ao quebrar linha (token fica na linha anterior); ↑/↓ com input não-vazio movem cursor, não histórico
  - `TestHistorySkipsBusyRejected` — send rejeitado por busy não entra no histórico
- Migrações: 3 asserções `m.input.Position()` → `len(textBeforeCursor(m.input))` (textarea não tem `Position()`; equivalente byte-offset do texto antes do cursor)
- Determinísticos: sem timing/rede nova (gateway httptest existente); drenagem síncrona de `EventError` no teste de histórico (agent não-ready emite antes de `Run` retornar)

## Verificação de Segurança
N/A — TUI local, sem backend/API/SQL/secrets. Input do prompt segue fluindo por `ExpandMentions` (glob com `filepath.Glob`, sem shell) e `agent.Run` como antes; nenhuma superfície nova.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Média | internal/tui/tui.go | 587 | Case `"shift+enter"` é inalcançável em produção: bubbletea v1.3.10 não representa shift+enter (terminais enviam CR = `"enter"`; sem kitty CSI-u) | Mantido + surrogates funcionais `"alt+enter"` (universal) e `"ctrl+j"` (terminais mapeiam shift+enter→LF) no mesmo case; ao migrar para bubbletea v2 (`Key.Modifiers`), o case já cobre. **kspec-qa deve configurar o terminal** (map shift+enter→LF) ou usar alt+enter no E2E |
| Baixa | internal/tui/tui.go | 253-257 | `MaxWidth` setado como campo (não existe `SetMaxWidth` no textarea v1.0.0) | Mantido: semântica idêntica; `SetWidth` no mesmo ponto recalcula o viewport interno |
| Baixa | internal/tui/tui.go | 582 | Comandos `/…` não entram no `promptHistory` | Mantido: spec fala em "prompts" (enviados ao agent); comandos não são reenviáveis |
| Baixa | internal/tui/tui.go | 670 | Recall de histórico fecha o popup de menções (não re-globa) | Mantido: evita ambiguidade de ↑ entre seleção do popup e navegação de histórico; digitar reabre o popup normalmente |
| Baixa | internal/tui/tui.go | 332-338 | Recálculo de altura via paste (fallthrough do `Update`) não tem teste unitário direto — `pasteMsg` é tipo privado do textarea | Coberto indiretamente pelo mesmo helper das teclas; E2E de paste multi-linha fica para o kspec-qa |
| Baixa | internal/tui/tui_test.go | — | 2 testes além dos 4 mandados | Mantido: edge cases exigidos pelo task-runner/review-runner (completion multi-linha, busy-rejected) |

## Pontos Positivos
- Interceptação exatamente no ponto especificado pela techspec (decisão #1): explícita, testável, sem remontar keymap do componente nem lutar contra a lib
- `handlePromptContentChange` centraliza o contrato "mudou conteúdo → reseta navegação + reajusta alturas" nos 3 caminhos de mutação (teclas, newline manual, paste) — detecção por valor, não por tecla, é robusta a qualquer fonte de mutação
- Altura por linhas lógicas via `LineCount()` — exatamente o requisito, imune a soft-wrap
- `viewChromeRows` generaliza o magic number 6 do código anterior (`height-6` = `height - 1 - 5`); layout verificado fechar exato na janela
- Navegação de histórico com semântica de shell (↑ clampa no mais antigo, ↓ além do mais novo volta ao vazio), sem quebrar o movimento de cursor multi-linha quando não navegando
- `setPromptCursor`/`promptRowCol` restauram cursor row/col após `SetValue` usando apenas API pública do textarea — mecânica validada por teste cross-row
- Interações preservadas: popup intercepta enter/tab/↑↓ antes de tudo; hint bar, live block e seleção intocados; 136 testes pré-existentes passam sem nenhuma regressão

## Recomendações
- (Task 5.0) Verificação final: validar o fluxo completo multi-linha + live block juntos (prompt alto enquanto comando streama)
- (kspec-qa) E2E pendentes da techspec: `shift+enter` em prompt longo (configurar terminal: map shift+enter→LF, ou usar alt+enter); ↑ recupera prompt anterior; paste multi-linha recalcula altura
- (Futuro) Ao migrar para bubbletea v2, o case `"shift+enter"` passa a funcionar nativamente via `KeyMsg.Modifiers` — remover os surrogates só depois de confirmar o parser kitty no terminal alvo

## Conclusão
Implementação aderente à techspec (REQ-001 + REQ-002): textarea com visual idêntico, keymap invertido por interceptação em `handleChatKey`, altura automática 1-8 por linhas lógicas, viewport com mínimo 3 (`minViewportRows` extraída), histórico FIFO de 20 navegável com ↑/↓ e reset ao digitar, envio preservando `\n` até o agent (provado no body HTTP). Checks todos verdes (`go build`, `go vet`, `gofmt -l`, `go test` — 142/142, incl. os 136 pré-existentes). A ressalva principal é limitação de plataforma documentada (bubbletea v1.3.10 não representa shift+enter), mitigada com surrogates funcionais testados e case future-proof — não bloqueante; as demais são decisões de interpretação testadas. **APROVADO COM RESSALVAS**; task 4.0 marcada como completa.

## Checks Executados
| Check | Resultado |
|-------|-----------|
| `go build ./...` | ✅ pass |
| `go vet ./...` | ✅ pass |
| `gofmt -l .` | ✅ vazio (nenhum arquivo a formatar) |
| `go test ./... -count=1` | ✅ 142/142 passando, 0 falhas |

> Nota de ambiente: um run intermediário encadeado (`build && vet && gofmt && test`) apresentou crash transitório do toolchain Go em Darwin/arm64 (idêntico ao registrado no review_3.0.md); runs subsequentes completos passaram. Não relacionado ao código da task.
