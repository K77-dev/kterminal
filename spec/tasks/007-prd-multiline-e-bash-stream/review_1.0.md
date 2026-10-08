# Relatório de Code Review - ExecuteStream com line writer no bash (Task 1.0, REQ-003)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados: 3 (`internal/tools/tools.go`, `internal/tools/bash.go`, `internal/tools/bash_test.go`)
- Linhas Adicionadas: ~160 (líquido, escopo da task: tools.go +12, bash.go +47, bash_test.go +101)
- Linhas Removidas: ~13 (líquido; refatoração do executor inline para `runBash` + wrapper)
- Nota: o working tree contém mudanças não commitadas das features 001-006; os números acima são do escopo desta task.

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Zero comentários nos 3 arquivos |
| Apenas stdlib | OK | Novos imports: `io` (bash.go), `reflect` (teste) — ambos stdlib |
| code-standards.md | N/A | Arquivo contém apenas o título (vazio) |
| architecture-ddd.md / tests.md / logging.md | N/A | Não aplicáveis a stack Go, conforme seção "Conformidade com Skills Padrões" da techspec |
| Padrões do repositório | OK | `testing` stdlib, estilo AAA, sem relógio necessário nesta task (não há throttle aqui) |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `ExecuteStream(ctx, name, argsJSON, onLine) (Result, error)` no `Registry` | SIM | Assinatura idêntica à techspec (tools.go:65) |
| `Execute` wrapper com `onLine = nil` | SIM | `return r.ExecuteStream(ctx, name, argsJSON, nil)` (tools.go:89-91); callers preservados (agent.go segue em `Execute`) |
| `onLine == nil` → comportamento idêntico, nenhuma alocação de line writer | SIM | Dispatch cai em `t.Execute` → `runBash(ctx, args, nil)`; line writer só é alocado no branch `onLine != nil` |
| `newLineWriter(onLine)`: acúmulo até `\n`, callback sem `\n`, flush final | SIM | bash.go:84-112; `Write` retorna `(len(p), nil)` (contrato do MultiWriter) |
| bash com `io.MultiWriter(buf, lineWriter)` quando `onLine != nil` | SIM | Com correção justificada — ver Problemas Encontrados #1 |
| Timeout 120s, exit status, truncamento 32k inalterados | SIM | Linhas de timeout/exit/truncamento byte-a-byte idênticas às anteriores |
| Outras tools ignoram `onLine` | SIM | Sem campo `ExecuteStream` → fallback para `Execute`; `fs.go` intocado (coerente com a tabela de arquivos da techspec) |
| Consolidado e stream consistentes por construção (decisão #3) | SIM | Uma única passada no pipe; MultiWriter alimenta buffer e line writer com os mesmos bytes na mesma ordem |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 1.1 `ExecuteStream` no `Registry` + `Execute` wrapper | COMPLETA | Dispatch `onLine != nil && t.ExecuteStream != nil`; mensagens de erro e normalização `(no output)` preservadas |
| 1.2 `newLineWriter` | COMPLETA | Acúmulo, split em `\n`, callback, flush do restante; idempotente |
| 1.3 bash no `io.MultiWriter` preservando timeout/truncamento | COMPLETA | MultiWriter único compartilhado entre Stdout/Stderr; flush após `cmd.Run()` (cópias do exec concluídas) |
| 1.4 Testes 1-5 da techspec | COMPLETA | 5 testes em `bash_test.go`, comandos exatamente os pinados na techspec |

## Testes
- Total de Testes: 127 (122 pré-existentes + 5 novos)
- Passando: 127
- Falhando: 0
- Coverage: 78.7% (`internal/tools`)
- Race detector: limpo (`go test -race ./internal/tools/`)
- Testes novos: `TestExecuteStreamEmitsLinesInOrder` (mandatório: `[1..10]` em ordem + consolidado com 10 linhas), `TestExecuteStreamMergesStdoutStderr` (stream e consolidado com linhas de ambos os fds), `TestExecuteStreamNilCallbackMatchesExecute` (paridade exata via `reflect.DeepEqual`), `TestExecuteStreamFinalLineWithoutNewline` (flush de cauda sem `\n`), `TestExecuteStreamTimeoutKillsProcess` (morto prontamente + erro de timeout + zero linhas)
- Edge cases cobertos: cauda sem `\n`, comando silencioso (nenhuma linha streamada), timeout com callback ativo, paridade nil vs `Execute`

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tools/bash.go | 57 | Desvio intencional do snippet literal da techspec: o snippet atribui `io.MultiWriter(buf, lineWriter)` duas vezes (duas instâncias) e incondicionalmente. Duas instâncias quebram a igualdade de interface em `os/exec` → dois pipes/dois goroutines de cópia → data race no `bytes.Buffer` e no `lineWriter`; instância única com `onLine == nil` (writer nil) causaria panic no primeiro `Write`. Implementado com UM MultiWriter compartilhado entre `cmd.Stdout` e `cmd.Stderr`, criado só quando `onLine != nil` — exatamente o que os requirements da task 1.0 determinam ("MultiWriter quando `onLine != nil`"; "nenhuma alocação" com nil). Verificado no fonte do `os/exec` (`childStderr` reusa o pipe de stdout quando interface-equal) e validado com `-race`. | Nenhuma — desvio correto e necessário; documentado aqui |

## Pontos Positivos
- Preservação exata do comportamento consolidado: `runBash` mantém timeout, kill por contexto, exit status e truncamento idênticos; `Execute` é wrapper puro.
- Design livre de race por construção: um único writer compartilhado serializa as escritas e produz merge verdadeiro stdout/stderr na ordem de chegada (mesma garantia que o código atual já dependia com `&buf`).
- Flush incondicional após `cmd.Run()` cobre também os caminhos de erro (timeout/exit não-zero) — nenhuma linha parcial perdida.
- Testes determinísticos (ordem e conteúdo exatos, sem assertions de timing frágeis); o mandatório replica o comando pinado da techspec.
- Blast radius mínimo: `fs.go` e demais tools intocados, coerente com a tabela de arquivos da techspec.

## Recomendações
- O caminho "flush de cauda parcial durante timeout" (ex.: `echo before; sleep 300`) é garantido estruturalmente (flush incondicional), mas não é assertado diretamente pelos testes 1-5 da techspec. Sugere-se exercitar essa interação nos testes de flush do agent na task 2.0.
- Na task 2.0, ao trocar o agent para `ExecuteStream`, manter `Execute` inalterado como wrapper (já é o caso).

## Conclusão
Implementação aderente à techspec e à task 1.0 em todos os requisitos, com um desvio deliberado e justificado do snippet literal (MultiWriter único) que corrige um race e um panic latentes no esboço, honrando a decisão #3 da techspec ("uma única passada no pipe"). Todos os checks passam (build, vet, gofmt, 127/127 testes, race limpo). APROVADO.
