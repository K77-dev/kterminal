# Relatório de Code Review — Diagnósticos pós-edição (go vet hook) — Task 2.0

## Resumo

- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados: 2 (`internal/tools/tools.go`, `internal/tools/tools_test.go`)
- Linhas Adicionadas: ~161 (21 em `tools.go` + ~140 em `tools_test.go`)
- Linhas Removidas: 0

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado nos dois arquivos |
| Sem dependências externas | OK | Único import novo é `strings` (stdlib) |
| Stack Go 1.27 / módulo `kterminal` | OK | Compila com go1.27.1; sem packages novos |
| Rules DDD/TS/Vitest/logging | N/A | Brownfield Go, conforme seção Conformidade com Skills Padrões da techspec |
| Padrões de teste do repositório | OK | `testing` + temp dirs + hook fake — os testes 9-11 não dependem do binário `go` instalado (requisito do PRD para o pacote `tools`); reusa `mustArgs` de `fs_test.go` |
| Hook injetado via setter (não no construtor) | OK | `NewRegistry` intacto; `SetOnGoEdit` é o único ponto de injeção desta task; `main.go` intocado (composição é a task 3.0) |

## Aderência à TechSpec

| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Campo `OnGoEdit func(path string) string` no `Registry`, nil default | SIM | `tools.go:35`; zero value de func é nil, `NewRegistry` não inicializa |
| `SetOnGoEdit(hook func(path string) string)` | SIM | `tools.go:54` — assinatura exata da techspec (Interfaces Principais) |
| Após sucesso da tool: `name == "write" \|\| name == "edit"` + `r.OnGoEdit != nil` + `strings.HasSuffix(path, ".go")` → `out += r.OnGoEdit(path)` | SIM | `goEditDiagnostics` (`tools.go:96-108`), mesma ordem de guards da spec; chamada em `tools.go:92` |
| Hook roda **após** a escrita ter sucesso — edição nunca revertida nem falha por causa dele | SIM | Chamada fica depois do `if err != nil { return ... }`; hook retorna apenas string (sem erro), a tool não conhece falha de diagnóstico; teste prova que edit falhado (arquivo inexistente) não dispara o hook |
| Ponto de chamada no fluxo consolidado, independente de `onLine` (interação techspec 007) | SIM | Único ponto de chamada em `ExecuteStream`, após o branch stream/não-stream; `Execute` delega; teste exercita `ExecuteStream` com `onLine` não-nil — espelha o caminho de produção (`agent.go:327` sempre passa `collector.onLine`) |
| Anexa ao `Output` do `Result` (interação techspec 002, assinatura pós-002) | SIM | `res.Output += ...`; `Diff` preservado (testado) |
| `write` em `.txt`, `edit` em `.md`, `read`/`bash` → hook não chamado | SIM | Teste 10 cobre os quatro casos, incluindo `read` em arquivo `.go` (prova o guard de nome, não só o de sufixo) |
| Registry sem hook (nil) → comportamento idêntico ao atual (noop) | SIM | `goEditDiagnostics` retorna `""` imediato; teste 11 com igualdade exata de output (`wrote N bytes to ...` / `edited ...`) |
| `main.go`: `reg.OnGoEdit = tools.GoVetHook` | N/A | Escopo da task 3.0, conforme Ordem de Construção (passos 3-4) |
| Teste 12 (`TestAgentSelfCorrectsWithinTurn`) | N/A | Escopo da task 3.0 (REQ-004) |

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 2.1 Campo `OnGoEdit` + setter `SetOnGoEdit` no `Registry` | COMPLETA | Campo exportado (a techspec mostra `main.go` atribuindo diretamente) + setter da interface |
| 2.2 Chamada do hook em `Execute` após sucesso de `write`/`edit` em `.go` (extraindo path dos args) | COMPLETA | Fluxo consolidado do `ExecuteStream`; path via `optStr(args, "path")` — args nil ou path não-string degradam para "" → sem hook (seguro por construção) |
| 2.3 Testes 9-11 da techspec com hook fake | COMPLETA | 3/3 implementados nominalmente, todos passando |

## Testes

- Total de Testes: 171 (168 pré-existentes + 3 novos)
- Passando: 171
- Falhando: 0
- Skips nesta máquina: 0 (os 3 novos usam hook fake — não dependem de Go instalado, requisito do PRD)
- Coverage: N/A (sem ferramenta de coverage configurada no projeto); branches exercitados: hook chamado (write e edit, via `Execute` e via `ExecuteStream` com `onLine`), hook não chamado (`.txt`, `.md`, `read` em `.go`, `bash`, tool falhada), nil hook noop (igualdade exata)
- Evidência nominal: `TestExecuteCallsHookOnGoEdit` PASS, `TestExecuteSkipsHookForNonGoOrOtherTools` PASS, `TestExecuteNilHookNoop` PASS

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tools/tools.go | 92 | O append do hook ocorre após a normalização `(no output)`; para `write`/`edit` o output nunca é vazio (`wrote ...`/`edited ...`), então não há interação prática — registro apenas como decisão de ordem documentada | Nenhuma — se uma tool customizada nomeada `write` retornasse output vazio, o resultado seria `(no output)\n\nDiagnostics: ...`, bem formado |
| Baixa | internal/tools/tools.go | 107 | Hook que entrasse em `panic` derrubaria o processo; fora do contrato da techspec (best-effort = só string, `GoVetHook` não entra em pânico) e `recover` não é especificado — não adicionado para evitar scope creep | Se hooks de terceiros passarem a ser injetados, considerar `defer recover` no ponto de chamada (evolução futura) |
| Baixa | internal/tools/tools_test.go | — | `tools_test.go` é untracked no working tree (herança das features 001-008 não commitadas); a contribuição desta task são os 3 testes novos + 4 imports — sem interferência nos testes existentes do arquivo (`TestRegistryPassesContextToExecutor` intacto) | Nenhuma — estado do working tree é intencional (sem commit, conforme instrução) |

## Pontos Positivos

- Ponto de chamada único no fluxo consolidado: `Execute` e `ExecuteStream` (com ou sem `onLine`) passam pelo mesmo código — a interação com a techspec 007 fica garantida estruturalmente, não por duplicação
- Teste 9 espelha o caminho de produção real: `agent.go:327` sempre chama `ExecuteStream` com `collector.onLine` não-nil; o teste usa essa forma para o `write` e `Execute` para o `edit`, cobrindo os dois pontos de entrada
- Guard de nome provado independentemente do guard de sufixo: `read` em arquivo `.go` no teste 10 isola a condição `name == "write" || name == "edit"`
- "Após sucesso" provado com cenário de erro: edit em arquivo inexistente falha e o hook não é chamado — a edição nunca é revertida nem o diagnóstico vaza para tool falhada
- Asserções fortes: `HasSuffix` exato do bloco appendado e igualdade exata de output no caso nil (prova byte a byte o comportamento inalterado)
- `Diff` do `Result` verificado intacto com hook ativo — o diagnóstico não clobbera o resultado
- Escopo cirúrgico: 21 linhas em `tools.go`, zero mudanças em `diagnostics.go`, `fs.go`, `main.go`, agent/TUI/session (reservados às tasks 1.0/3.0)

## Recomendações

- Na task 3.0, injetar via `reg.SetOnGoEdit(tools.GoVetHook)` ou atribuição direta `reg.OnGoEdit = tools.GoVetHook` (ambos suportados; a techspec menciona a atribuição direta no `main.go`)
- O teste 12 (task 3.0) deve capturar a 2ª chamada ao gateway com a mensagem `role: "tool"` contendo o bloco `Diagnostics:` — o contrato de formato (`\n\nDiagnostics:...` como sufixo do output) está congelado pelos testes 9-11

## Verificação

- `go build ./...` — OK
- `go vet ./...` — OK
- `gofmt -l .` — sem output (OK; verificado isoladamente, pois `gofmt -l` sempre retorna exit 0)
- `go test ./... -count=1` — ok em todos os pacotes (171/171; `internal/tools` ~33s, dominado pelo teste de timeout de 30s da task 1.0, já documentado na review_1.0)

## Conclusão

Implementação aderente à techspec e à task 2.0: o `Registry` ganha o campo injetável `OnGoEdit` (nil default) com setter, e o ponto de chamada roda no fluxo consolidado do `ExecuteStream` após sucesso de `write`/`edit` em `.go`, com os três guards da spec na ordem exata. Os 3 testes exigidos (9-11) passam nominalmente, cobrindo caminho feliz (write e edit, `Execute` e `ExecuteStream` com `onLine`), casos negativos (`.txt`/`.md`/`read`/`bash`/tool falhada) e noop com hook nil (igualdade exata). Os 168 testes pré-existentes permanecem intactos. Sem ressalvas bloqueantes — as três ocorrências listadas são de severidade baixa e de natureza documental. Aprovado; task 2.0 pode ser marcada completa.
