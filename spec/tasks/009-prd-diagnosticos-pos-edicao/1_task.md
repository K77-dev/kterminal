# Tarefa 1.0: `GoVetHook` e detecção de módulo em `internal/tools/diagnostics.go`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Hook pós-edição em arquivos Go (detecção de módulo)
- REQ-002 — Diagnóstico best-effort com `go vet`
- REQ-003 — Feedback explícito ao modelo (formatação do bloco)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

O motor de diagnóstico, isolado e testável: o novo arquivo `internal/tools/diagnostics.go` implementa `goModuleRoot(path)` (sobe diretórios a partir do arquivo procurando `go.mod`) e `GoVetHook(path) string` — roda `go vet` no pacote do arquivo editado com timeout de 30s (`GOFLAGS=-mod=mod`), filtra a saída combinada para as linhas do arquivo editado (foco — sem ruído do repositório) e formata o bloco `Diagnostics:` com as linhas de erro, ou `Diagnostics: clean` como feedback explícito de sucesso. Best-effort como contrato: `GoVetHook` **nunca** retorna erro — Go ausente no PATH, timeout, rede ou erro de ambiente devolvem string vazia; a edição em si nunca falha por causa do hook (requisito não negociável do PRD).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`os/exec`, `context`, `path/filepath`, `strings`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; casos que exigem binário `go` pulados com `exec.LookPath` guard + `t.Skip`.
</skills>

<requirements>
- Constante `vetTimeout = 30 * time.Second`.
- `goModuleRoot(path) (root, rel string, ok bool)`: a partir de `filepath.Abs(filepath.Dir(path))`, sobe um diretório por vez procurando `go.mod`; sem `go.mod` até a raiz → `ok=false` (fora de módulo não roda — REQ-001).
- `GoVetHook(path) string` — nunca retorna erro:
  1. Sem módulo → `""`.
  2. `exec.LookPath("go")` falha → `""` (ambiente sem Go — REQ-002).
  3. `exec.CommandContext(ctx, "go", "vet", pkgPattern)` com ctx de 30s, `cmd.Dir = root`, env `GOFLAGS=-mod=mod`; `pkgPattern = "./" + filepath.Dir(rel)` normalizado (arquivo na raiz do módulo → `"."`).
  4. Saída combinada (stdout+stderr) filtrada por linhas contendo o caminho relativo do arquivo (separador do OS) — só o arquivo editado (foco do PRD).
  5. Formatação: linhas filtradas → `"\n\nDiagnostics:\n" + linhas`; nenhuma → `"\n\nDiagnostics: clean"` (REQ-003).
  6. Timeout → processo morto via contexto, `""`; exit status com linhas do arquivo → as linhas (caso normal); exit status sem linhas do arquivo (ambiente/build de outro pacote) → `""`.
- Nenhum input do modelo chega à linha de comando do `go` — `pkgPattern` é derivado de caminho validado.
</requirements>

## Subtarefas

- [ ] 1.1 Implementar `goModuleRoot` (subida de diretórios procurando `go.mod`)
- [ ] 1.2 Implementar `GoVetHook` com LookPath guard, vet com timeout 30s, `GOFLAGS=-mod=mod` e filtro por caminho relativo
- [ ] 1.3 Implementar a formatação do bloco (`Diagnostics:` com linhas / `Diagnostics: clean` / vazio)
- [ ] 1.4 Escrever `internal/tools/diagnostics_test.go` com módulos temporários (testes 1-8 da techspec)

## Detalhes de Implementação

- Regras numeradas do `GoVetHook` e do `goModuleRoot`: techspec, seção **Design de Implementação** (subseção "Regras do `GoVetHook`").
- Formato do bloco (contrato com o modelo): techspec, seção **Design de Implementação → Modelos de Dados**.
- Trade-off `GOFLAGS=-mod=mod` (pode alterar `go.mod`/`go.sum`): techspec, seção **Verificações Técnicas → Segurança** e **Considerações Técnicas → Decisões Principais** (item 5).
- Decisões "Diagnostics: clean explícito" e "filtro por caminho relativo": techspec, seção **Considerações Técnicas → Decisões Principais** (itens 2, 3).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste mandatório prova: temp module com `undefined: Foo` → hook retorna bloco `Diagnostics:` contendo a linha com o caminho relativo.
- Testes provam: arquivo válido → `Diagnostics: clean`; sem `go.mod` → `""`; erro em outro arquivo filtrado → `clean`; timeout → `""` dentro do limite; sem `go` no PATH → `""` imediato.
- Os testes do pacote passam em máquina sem Go instalado (skips condicionais).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tools/diagnostics_test.go`, temp dirs; casos que exigem `go` pulados com `exec.LookPath` guard + `t.Skip` — techspec **Abordagem de Testes → Testes Unidade**, itens 1-8):
  - `TestGoModuleRootFound` — temp dir com `go.mod` + `internal/x/a.go` → root e rel corretos.
  - `TestGoModuleRootNotFound` — fora de módulo → ok=false.
  - `TestVetHookReturnsDiagnostic` (mandatório) — `undefined: Foo` → bloco `Diagnostics:` com a linha do arquivo.
  - `TestVetHookCleanOnValidFile` — arquivo válido → `Diagnostics: clean`.
  - `TestVetHookNoModuleEmpty` — sem `go.mod` → `""`.
  - `TestVetHookFiltersOtherFiles` — erro em `other.go` filtrado → `clean`.
  - `TestVetHookTimeoutKillsProcess` — script lento via PATH do teste → `""` dentro do timeout + margem.
  - `TestVetHookNoGoInPath` — `PATH` vazio → `""` imediato.
- [ ] Testes de integração — fora do escopo da task; a chamada pelo registry é validada na task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/diagnostics.go` — novo: `GoVetHook`, `goModuleRoot`, filtro e formatação
- `internal/tools/diagnostics_test.go` — novo: módulos temporários, diagnóstico/clean/timeout/sem-go
