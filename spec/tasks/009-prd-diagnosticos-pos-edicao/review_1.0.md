# Relatório de Code Review — Diagnósticos pós-edição (go vet hook) — Task 1.0

## Resumo

- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 2 (novos: `internal/tools/diagnostics.go`, `internal/tools/diagnostics_test.go`)
- Linhas Adicionadas: 247 (89 + 158)
- Linhas Removidas: 0

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário nos dois arquivos |
| Sem dependências externas | OK | Apenas stdlib (`context`, `os`, `os/exec`, `path/filepath`, `regexp`, `strings`, `time`) |
| Stack Go 1.27 / módulo `kterminal` | OK | Compila com go1.27.1; sem packages novos fora de `internal/tools` |
| Rules DDD/TS/Vitest/logging | N/A | Brownfield Go, conforme seção Conformidade com Skills Padrões da techspec |
| Padrões de teste do repositório | OK | `testing` + temp dirs + `t.Helper()` + `t.Skip` com `exec.LookPath` guard, seguindo o estilo de `bash_test.go`/`fs_test.go` |

## Aderência à TechSpec

| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `const vetTimeout = 30 * time.Second` | SIM | |
| `goModuleRoot(path) (root, rel, ok)` — sobe de `filepath.Abs(filepath.Dir(path))` procurando `go.mod` | SIM | Para na raiz do filesystem sem `go.mod` → `ok=false`; retorna rel do **arquivo** |
| `GoVetHook` nunca retorna erro (best-effort) | SIM | Só retorna string; nenhum caminho propaga erro |
| Sem módulo → `""` (REQ-001) | SIM | Teste 5 |
| `exec.LookPath("go")` falha → `""` (REQ-002) | SIM | Teste 8 (PATH vazio, retorno imediato) |
| `exec.CommandContext` com ctx 30s, `cmd.Dir = root`, `GOFLAGS=-mod=mod` | SIM | `vetEnv()` remove `GOFLAGS` pré-existente antes de aplicar, garantindo env efetiva `-mod=mod` |
| `pkgPattern = "./" + filepath.Dir(rel)` normalizado; arquivo na raiz → `"."` | SIM | Verificado empiricamente: `go vet ./internal/x` e `go vet .` funcionam a partir do root |
| Saída combinada filtrada por linhas contendo o caminho relativo (separador do OS) | SIM | Formato real do vet confirmado empiricamente: `vet: internal/x/broken.go:4:9: undefined: Foo` contém `rel` |
| Formatação `"\n\nDiagnostics:\n" + linhas` / `"\n\nDiagnostics: clean"` (REQ-003) | SIM | Testes 3 e 4 com asserção exata do bloco |
| Timeout → processo morto via contexto, `""` | SIM | Teste 7: fake `go` via PATH do teste, morto em ~30s, sem processos órfãos (`exec /bin/sleep`) |
| Exit status com linhas do arquivo → as linhas (caso normal) | SIM | Teste 3 (mandatório) |
| Exit status sem linhas do arquivo (erro de ambiente) → `""` | SIM | Discriminador `vetDiagPattern` (`\.go:\d+`): sem linhas de diagnóstico na saída → `""` (ex.: "no Go files", falha de rede, GOFLAGS inválido) |
| Nenhum input do modelo na linha de comando do `go` | SIM | `pkgPattern` derivado de caminho validado, não de texto livre |
| Hook injetado via setter no Registry / `main.go` | N/A | Escopo da task 2.0/3.0 — esta task entrega apenas o motor isolado, conforme a Ordem de Construção (item 1) |

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 1.1 `goModuleRoot` (subida procurando `go.mod`) | COMPLETA | Testes 1 e 2 |
| 1.2 `GoVetHook` com LookPath guard, timeout 30s, `GOFLAGS=-mod=mod`, filtro por caminho relativo | COMPLETA | Testes 3-8 |
| 1.3 Formatação do bloco (`Diagnostics:` / `clean` / vazio) | COMPLETA | Testes 3, 4, 5, 7, 8 |
| 1.4 `diagnostics_test.go` com módulos temporários (testes 1-8 da techspec) | COMPLETA | 8/8 testes implementados nominalmente, todos passando |

## Testes

- Total de Testes: 168 (160 pré-existentes + 8 novos)
- Passando: 168
- Falhando: 0
- Skips nesta máquina: 0 (guards `skipWithoutGo` ativos em 3, 4, 6, 7 para máquinas sem Go; 1, 2, 5, 8 não dependem do binário `go`)
- Coverage: N/A (sem ferramenta de coverage configurada no projeto; cobertura de branches do hook exercitada pelos 8 testes: módulo/não-módulo, go/não-go, diagnóstico/clean/filtrado, timeout)

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tools/diagnostics.go | 42 | Interpretação de conflito interno da spec: a regra 6 da techspec diz "exit status sem linhas do arquivo → `""`", mas o teste 6 (mandatório, listado na task e na techspec) exige "erro em outro arquivo → `Diagnostics: clean`". Resolvido com o discriminador `vetDiagPattern`: exit ≠ 0 sem linhas do arquivo editado → `clean` quando o vet emitiu linhas de diagnóstico para outros arquivos (foco do PRD, decisão 3: erros pré-existentes de outros arquivos não viram ruído), `""` quando não há nenhuma linha de diagnóstico (erro de ambiente). Efeito colateral documentado: "build de outro pacote" (dependência quebrada) devolve `clean` em vez do `""` literal da regra 6 — nenhum teste da techspec cobre esse caso; comportamento alinhado ao espírito do PRD (o arquivo editado está limpo) | Se a letra da regra 6 for considerada vinculante para o caso de dependência quebrada, restringir o discriminador a linhas do mesmo diretório do arquivo editado (task futura) |
| Baixa | internal/tools/diagnostics_test.go | 143 | `TestVetHookTimeoutKillsProcess` consome ~30s de suite (inerente ao `vetTimeout` const de 30s exigido pela spec) | Aceito pela spec; se a suite precisar acelerar, transformar `vetTimeout` em var de pacote sobrescrita no teste (mudança de spec) |
| Baixa | internal/tools/diagnostics.go | 15 | `regexp` não constava na lista ilustrativa de imports da task (`os/exec`, `context`, `path/filepath`, `strings`), mas é stdlib e necessário ao discriminador | Nenhuma — dentro da restrição "sem dependências externas" |

## Pontos Positivos

- Contrato best-effort respeitado à risca: nenhum caminho de `GoVetHook` retorna erro; a edição nunca falha por causa do diagnóstico (requisito não negociável do PRD)
- Filtro validado empiricamente antes da implementação: formato real do `go vet` (`vet: internal/x/broken.go:4:9:` e `./main.go:4:10:`) contém o caminho relativo, confirmando o design `Contains(rel)` da techspec
- Teste de timeout sem processos órfãos: o fake `go` usa `exec /bin/sleep`, então o processo morto pelo contexto é o filho direto
- `vetEnv()` elimina `GOFLAGS` pré-existente antes de aplicar `-mod=mod`, evitando ambiguidade de chaves duplicadas no env
- Asserções exatas (`== "\n\nDiagnostics: clean"`) nos testes de formatação, não apenas `Contains`
- Zero interferência nos 160 testes existentes; escopo restrito aos 2 arquivos da task (Registry/main.go intactos para as tasks 2.0/3.0)

## Recomendações

- Na task 2.0, ao injetar o hook no `Registry`, manter o guard `strings.HasSuffix(path, ".go")` no ponto de chamada conforme a techspec — o hook em si não valida extensão (por design, responsabilidade do caller)
- Considerar, em evolução futura, restringir o discriminador de "clean" ao diretório do pacote editado se o caso "dependência quebrada" passar a exigir silêncio

## Verificação

- `go build ./...` — OK
- `go vet ./...` — OK
- `gofmt -l .` — sem output (OK)
- `go test ./...` — ok em todos os pacotes (168/168; `internal/tools` 33.5s, dominado pelo teste de timeout de 30s da spec)

## Conclusão

Implementação aderente à techspec e à task 1.0: `goModuleRoot` e `GoVetHook` entregam o motor de diagnóstico isolado e testável, com os 8 testes exigidos passando (incluindo o mandatório `undefined: Foo` → bloco `Diagnostics:` com o caminho relativo) e os 160 testes pré-existentes intactos. O único ponto de interpretação — o conflito interno entre a regra 6 e o teste 6 da própria techspec — foi resolvido em favor do PRD (foco no arquivo editado + feedback explícito) e está documentado como ressalva não bloqueante, sem impacto em nenhum teste especificado. Aprovado com ressalvas; task 1.0 pode ser marcada completa.
