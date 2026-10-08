# Relatório de Code Review — 005 @menções de arquivo: Task 2.0 (Expansão no envio, chat com texto original e aviso inline)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal (working tree com mudanças não commitadas de 001–004 e 005/1.0 — diff cumulativo; escopo abaixo refere-se apenas ao incremental da task 2.0)
- Status: APROVADO
- Arquivos Modificados (escopo task 2.0): 2 (`internal/tui/tui.go`, `internal/tui/tui_test.go`)
- Linhas Adicionadas (escopo task 2.0): ~174 (8 em `tui.go`; ~166 em `tui_test.go`: 2 testes novos + 3 helpers + imports)
- Linhas Removidas (escopo task 2.0): 1 (`m.agent.Run(value)` → `m.agent.Run(expanded)`)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Receivers por valor na TUI | OK | `handleChatKey`/`View` mantêm value receiver; `mentionWarnings` mutado na cópia retornada (padrão Bubble Tea) |
| `strings.Builder` por ponteiro | OK | Nenhum builder novo por valor; loop de warnings escreve no builder local existente de `View` |
| Stack Go 1.27, módulo `kterminal`, stdlib | OK | Imports novos apenas nos testes (`io`, `os`, `path/filepath`, `kterminal/internal/session`) |
| Zero mudanças em `internal/agent` / `internal/session` | OK | `grep -i mention` em ambos os pacotes: zero ocorrências — requisito não negociável preservado |
| DDD/TS/Vitest | N/A | Brownfield Go, conforme techspec |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| No envio: `expanded, warnings := ExpandMentions(input)` antes de `agent.Run` (Fluxo de dados item 2) | SIM | No caminho de envio do Enter, após checks de vazio/comando/busy (pré-condições de envio); comandos `/...` não passam por expansão (correto: não vão ao agente) |
| Bloco do usuário no chat renderiza só o texto original | SIM | `userBoxStyle.Render(value)` com o valor original; teste prova que o chat não contém bloco nem conteúdo expandido |
| `agent.Run(expanded)` — agente alheio a menções (Decisão 1, não negociável) | SIM | Validado por captura dupla: evento `user` do transcript JSONL (igualdade exata) e body do request no gateway de teste |
| Transcript grava o expandido no evento `user` (REQ-004, automático por construção) | SIM | Zero mudanças em `agent.go`/`session.go`; `transcriptUserContent` compara Content com o expandido completo |
| Warnings inline em `colWarning` acima do prompt, sem bloquear o envio (Fluxo de dados item 3) | SIM | `warningStyle` (colWarning) renderizado entre viewport e prompt box; envio prossegue (`busy=true`, turno completa apesar do aviso) |
| Popup (`mentionPrefix`, Tab/Enter/Esc) | NÃO (escopo da task 3.0) | Conforme nota do review 1.0: techspec inconsistente sobre mentionPrefix; task 2.0 cuida apenas do fluxo de envio |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 2.1 Chamar `ExpandMentions` no ponto de envio, antes de `agent.Run` | COMPLETA | `handleChatKey` case "enter" |
| 2.2 Renderizar o bloco do usuário com o texto original | COMPLETA | Chat com tokens intactos; expandido nunca vaza para o chat |
| 2.3 Renderizar warnings inline em `colWarning` acima do prompt | COMPLETA | Loop em `View()`; posição validada por índices (chat < warning < fade) |
| 2.4 Testes 12–13 da techspec com captura do input do agent | COMPLETA | `TestSendExpandsButChatShowsOriginal` + `TestWarningRendersInline` |

## Testes
- Total de Testes: 99 (97 pré-existentes + 2 novos)
- Passando: 99
- Falhando: 0
- Coverage: não medida (sem tool de coverage configurado no projeto); cobertura comportamental dos requisitos da task: completa
- Comandos: `go build ./...` OK · `go vet ./...` OK · `gofmt -l .` vazio · `go test ./... -count=1` OK · `go test ./internal/tui/ -race` OK

### Testes novos (techspec, itens 12–13)
- `TestSendExpandsButChatShowsOriginal` (unidade + integração TUI→agent): envia `@a.txt o que é?` com temp dir como cwd; prova (a) chat contém só o original, sem bloco `--- Arquivo @a.txt ---` e sem conteúdo do arquivo; (b) evento `user` do JSONL igual ao expandido completo (igualdade exata de string); (c) request capturado no gateway contém o bloco expandido — o que o modelo efetivamente viu; (d) ciclo busy true→false.
- `TestWarningRendersInline`: 6 menções → aviso `max 5 file mentions` com ANSI de `colWarning` (`\x1b[38;2;245;167;65m`), posicionado entre o chat e o prompt (chat < warning < fade), envio não bloqueado (turno completa com TurnDone), aviso limpo no envio seguinte sem menções (ciclo de vida coberto).
- Helpers: `captureGateway` (canal buffered para o body — sincronização limpa, validada com `-race`), `newSessionAgentModel` (agent real com `session.Writer` em `XDG_DATA_HOME` temp), `transcriptUserContent` (parse do JSONL).

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tui/tui_test.go | asserção ANSI | Escape esperado `245;167;65` diverge do hex `#f5a742` (B=66) — truncamento float→uint8 do termenv v0.16.0 (`(66/255)*255 = 65.999…`) | Nenhuma ação: determinístico na versão pinada; segue a convenção dos 3 testes existentes que hardcodeiam ANSI exato |
| Baixa | internal/tui/tui.go | View() | Ciclo de vida do warning (limpo/substituído a cada envio) não é especificado na techspec; `/clear` não limpa o aviso | Opcional para a task 4.0 (verificação integrada): decidir se `/clear` deve limpar `mentionWarnings`; comportamento atual é previsível e coberto por teste |

Nenhum problema de severidade Alta/Média.

## Pontos Positivos
- Mudança cirúrgica no fluxo de envio: 8 linhas em `tui.go`, zero mudanças em `internal/agent`/`internal/session` — o contrato do agente permanece intocado (requisito não negociável).
- Captura dupla do "que o agente recebeu" (transcript JSONL + body do gateway) torna o teste à prova de regressão ponta a ponta, sem introduzir interfaces/mocks artificiais no código de produção.
- Teste do aviso cobre além do caminho feliz: posição na tela, cor exata, não-bloqueio do envio e ciclo de vida (limpeza no envio seguinte).
- Reuso total de `ExpandMentions` (1.0), `warningStyle` e do harness existente (`newTestModel`/`step`/`waitForAgentEvent`/`contentChunks`).

## Recomendações
- Na task 4.0 (verificação final), avaliar limpeza de `mentionWarnings` no `/clear`.
- E2E (kspec-qa): `@main.go o que esse arquivo faz?` respondendo sem tool call de leitura; menção inexistente chegando intacta ao agente.

## Conclusão
APROVADO. A implementação adere à techspec à risca: expansão no ponto de envio, chat fiel ao texto original, transcript gravando o expandido por construção e aviso inline em `colWarning` sem bloquear o envio. Os 99 testes passam (incluindo os 97 pré-existentes), build/vet/gofmt limpos, `-race` ok. As duas observações de severidade baixa são não bloqueantes e registradas para a task 4.0.
