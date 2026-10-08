# Relatório de Code Review — 005 @menções de arquivo: Task 3.0 (Popup de autocomplete de arquivos)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal (working tree com mudanças não commitadas de 001–004 e 005/1.0+2.0 — diff cumulativo; escopo abaixo refere-se apenas ao incremental da task 3.0)
- Status: APROVADO
- Arquivos Modificados (escopo task 3.0): 3 (`internal/tui/tui.go`, `internal/tui/theme.go`, `internal/tui/tui_test.go`)
- Linhas Adicionadas (escopo task 3.0): ~396 (~109 em `tui.go`: estado, key handling, glob, helpers, render; 12 em `theme.go`: 3 estilos; ~275 em `tui_test.go`: testes 8-11 da techspec)
- Linhas Removidas (escopo task 3.0): 0

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Receivers por valor na TUI | OK | `handleChatKey`/`mentionPopupView` mantêm value receiver; mutadores (`closeMentionPopup`, `refreshMentionSuggestions`, `completeMention`) com pointer receiver sobre cópias locais — mesmo padrão dos existentes `refreshContent`/`scrollUp` |
| `strings.Builder` por ponteiro | OK | Builders locais em `mentionPopupView`/`View`; nenhum builder por valor no Model |
| Sem estado global | OK | Popup vive no Model (`mentionOpen`, `mentionItems`, `mentionSelected`) |
| Stack Go 1.27, módulo `kterminal`, stdlib | OK | Imports novos apenas `path/filepath` e `sort` — exatamente os previstos na techspec |
| TUI não importa `internal/tools` | OK | Direção de dependência inalterada |
| DDD/TS/Vitest | N/A | Brownfield Go, conforme techspec |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Estado do popup no Model: `mentionOpen bool`, `mentionItems []string`, `mentionSelected int` | SIM | Campos adicionados junto a `mentionWarnings`; invariantes: aberto ⇒ itens não vazios; seleção sempre resetada ao (re)abrir |
| `mentionPrefix` recebe o texto ATÉ O CURSOR (nota do review 1.0) | SIM | `textBeforeCursor` fatia o valor por runas na posição do cursor (`input.Position()` é índice de runa em bubbles v1.0.0) e alimenta `mentionPrefix` — assinatura da task 1.0 inalterada |
| Sugestões: `filepath.Glob(prefix + "*")`, arquivos apenas, ordenados, máx 10 | SIM | `mentionSuggestions`: glob do prefixo, `sort.Strings`, `os.Stat` filtra diretórios, cap em `maxMentionSuggestions` (10) |
| `Tab` completa com o item selecionado (substitui o token no input); popup fechado | SIM | Intercept no topo de `handleChatKey`; `completeMention` substitui o token sob o cursor e reposiciona o cursor após o caminho |
| `Enter` com popup aberto completa **sem enviar**; com popup fechado envia | SIM | Intercept anterior ao switch principal; teste prova `busy=false` + nenhum body no gateway, e o segundo Enter envia com a menção expandida |
| `Esc` fecha o popup sem cancelar turno | SIM | Intercept com precedência sobre o cancelamento: fecha só o popup mesmo com turno em execução (segundo Esc cancela); teste determinístico via `captureGateway` prova que o turno completa (`TurnDone`) |
| `↑`/`↓` movem a seleção; digitação re-glob | SIM | Navegação com wrap-around consumida pelo popup (não move o cursor do input); `refreshMentionSuggestions` após todo `input.Update` em chat |
| Completar substitui o token sob o cursor, não o fim do input | SIM | Teste com cursor no meio: `"olha @b fim"` + Left×4 + Tab → `"olha @b.txt fim"`, cursor após o token |
| Renderização entre viewport e prompt box; itens `colTextMuted`, selecionado `colPrimary`; paleta existente | SIM | Popup renderizado após warnings e antes do prompt; teste valida posição por índices (chat < popup < prompt) e ANSI das duas cores |
| Sem cores novas | SIM | Painel usa `colBorder`/`colBgPanel` existentes; itens `colTextMuted`/`colPrimary` |

## Decisões desta Task (delegadas pela task/nota do review 1.0)
| Decisão | Escolha | Justificativa |
|---------|---------|---------------|
| Glob com prefixo vazio (`@` sozinho) | `filepath.Glob("*")` — arquivos do cwd | Nota do review 1.0 delegava explicitamente; comportamento uniforme da fórmula `prefix+"*"`; teste `TestPopupListsFilesOnAtSign` |
| Token sem matches / padrão de glob inválido (`ErrBadPattern`, ex.: `@a[`) | Popup fechado | Nunca renderiza popup vazio; ambos os ramos testadas |
| `↑`/`↓` | Wrap-around | Padrão em autocomplete de terminais agênticos; teste cobre wrap nas duas direções |
| `Esc` com popup aberto + turno em execução | Fecha só o popup (não cancela) | Leitura literal do requisito "sem cancelar turno"; convenção de modal aninhado — segundo Esc cancela; testado |
| Painel do popup | `RoundedBorder` + `colBorder` + `colBgPanel` | Techspec especifica cores dos itens e posição; painel dá leitura de "popup" usando apenas paleta existente |
| Pós-completação | Popup fecha sem re-glob | Evita reabrir com o próprio arquivo completado como única sugestão |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 3.1 Estado do popup no Model alimentado via `mentionPrefix` + glob a cada mudança de input | COMPLETA | `refreshMentionSuggestions` após todo `input.Update` em chat; re-glob também ao mover o cursor (Left/Right) |
| 3.2 Key handling: Tab completa, Enter completa sem enviar, Esc fecha, ↑/↓ movem seleção | COMPLETA | Intercept no topo de `handleChatKey`; demais teclas (incl. `ctrl+c`) caem para o fluxo normal |
| 3.3 Renderizar o popup entre viewport e prompt box com a paleta existente | COMPLETA | `mentionPopupView` + bloco no `View`; posição e cores validadas por teste |
| 3.4 Testes 8-11 da techspec | COMPLETA | 4 testes novos, todos passando |

## Testes
- Total de Testes: 103 (99 pré-existentes + 4 novos)
- Passando: 103
- Falhando: 0
- Coverage (pacote `internal/tui`): 78.9%; funções novas: `closeMentionPopup`/`refreshMentionSuggestions`/`mentionPopupView` 100%, `mentionSuggestions` 92.3%, `completeMention` 83.3%, `textBeforeCursor` 80% (linhas restantes são guardas defensivas de estados inalcançáveis via fluxo público)
- Novos (techspec itens 8-11):
  - `TestPopupListsFilesOnAtSign` — `@` → popup com arquivos do cwd (máx 10, ordenados, sem diretórios); posição/cores no view; navegação ↑/↓ com wrap sem mover o cursor; re-glob ao digitar com reset de seleção; edge cases: prefixo sem matches e padrão de glob inválido
  - `TestTabCompletesFirstResult` — Tab completa o primeiro resultado e fecha o popup; cursor no meio do texto substitui o token sob o cursor; Tab completa o item selecionado (não só o primeiro)
  - `TestEnterWithPopupCompletesNotSends` — Enter com popup completa sem chamar `agent.Run` (busy=false + nenhum body capturado); Enter com popup fechado envia e o body contém a menção expandida
  - `TestEscClosesPopup` — Esc fecha sem cancelar turno (idle: estado/busy/input/blocks/viewport intactos; busy: turno completa com `TurnDone`); digitar reabre o popup
- Comportamento real verificado: estado do Model, valor e posição do cursor do input, ANSI da paleta, ordem de renderização, body HTTP capturado — não apenas "executa sem erro"
- Nenhum teste existente quebrou (99 pré-existentes passando, incl. `TestEscCancelsWhenBusy` e `TestEscNoOpWhenIdle`)

## Verificação de Segurança
- N/A — TUI local sem backend/API/endpoint. Glob/Stat operam sobre input do próprio usuário no próprio cwd (mesmo poder de digitar o caminho); sem secrets, sem dados sensíveis em logs.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tui/tui.go | View | Popup empilhado pode exceder a altura do terminal em janelas baixas (viewport não encolhe) — comportamento literal da techspec ("entre viewport e prompt box") | Futuro: sobrepor o popup reduzindo a altura do viewport enquanto aberto (fora do escopo da task) |
| Baixa | internal/tui/tui.go | mentionPopupView | Caminhos longos não são truncados no popup (cosmético, não especificado) | Futuro: truncar itens à largura do terminal |

Nenhum problema de severidade Alta/Média.

## Pontos Positivos
- `mentionPrefix` da task 1.0 consumido sem modificação: `textBeforeCursor` fornece o texto até o cursor e o token fica por construção no fim do slice, permitindo completar com `before[:len(before)-len(prefix)-1]` sem duplicar a lógica de boundary
- Invariantes claras (aberto ⇒ itens; seleção resetada) mantidas por um único ponto (`closeMentionPopup` + `refreshMentionSuggestions`)
- Intercept do popup isolado no topo de `handleChatKey` — fluxo de envio da task 2.0 intocado
- Testes cobrem além do caminho feliz: wrap, cursor no meio, busy, re-glob pós-Esc, sem matches, glob inválido, seleção resetada

## Recomendações
- Na task 4.0 (verificação final), considerar o truncamento de itens longos no popup e o comportamento em terminais baixos como itens de polimento (não bloqueantes)
- E2E do fluxo Tab (deferido para `kspec-qa` conforme a task)

## Conclusão
APROVADO. A implementação segue a techspec à risca (estado no Model, glob `prefix+"*"` com arquivos apenas/ordenados/máx 10, Tab/Enter completam sem enviar, Esc fecha sem cancelar, ↑/↓ navegam, render entre viewport e prompt com paleta existente), as decisões delegadas à task foram tomadas e documentadas, os 4 testes exigidos cobrem caminho feliz e edge cases, e todos os checks passam: `go build ./...`, `go vet ./...`, `gofmt -l .` (vazio) e `go test ./...` com 103/103 passando (99 pré-existentes + 4 novos).
