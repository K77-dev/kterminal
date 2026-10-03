# 09 — Diagnósticos pós-edição (go vet hook)

## Contexto

kterminal é um terminal agêntico em Go. O agente edita arquivos com `write`/`edit` e só descobre que quebrou o código quando o usuário reclama. Um LSP completo é infra pesada; para um primeiro passo, um hook de diagnóstico pós-edição com `go vet` dá retorno imediato ao agente no mesmo turno.

Arquitetura relevante:

- `internal/tools/fs.go` — tools `write` e `edit` (mutating).
- `internal/tools/tools.go` — `Registry` com `Tool.Mutating`.
- `internal/agent/agent.go` — o resultado do tool volta ao LLM como mensagem `role: "tool"`; o modelo reage ao texto.

## Objetivo

Depois de toda edição em arquivo `.go`, rodar `go vet` no pacote afetado e anexar o diagnóstico ao resultado do tool — o próprio modelo corrige o erro no passo seguinte, sem intervenção humana.

## Especificação

1. `Registry` ganha um hook: `OnGoEdit func(path string) string` — chamado após `write`/`edit` bem-sucedidos em arquivos `.go` (só quando o arquivo está dentro de um módulo Go — detectar `go.mod` subindo diretórios).
2. Implementação do hook em `internal/tools/diagnostics.go`:
   - `go vet ./<pacote-do-arquivo>` executado no diretório do módulo (timeout 30s, `GOFLAGS=-mod=mod` para não travar em downloads).
   - Filtrar a saída para o arquivo editado (linhas com o caminho relativo do arquivo); se `go vet` falhar por razões de ambiente (sem go no PATH, rede), retornar string vazia silenciosamente — diagnóstico é best-effort.
   - Formato do retorno: `\n\nDiagnostics:\n<linhas>` anexado ao output do tool. Sem diagnostics → anexar `\n\nDiagnostics: clean` (feedback explícito para o modelo).
3. Fallback rápido: se `go vet` demorar mais que 30s, matar e retornar só o output original.
4. O hook é injetado no `Registry` pelo `main.go` (mantém o pacote tools testável sem depender de Go instalado).
5. TUI: nada especial — o diagnóstico aparece no `EventToolResult` como já acontece; truncamento normal se aplica.
6. Transcript: o diagnóstico já vai no `Result` do evento `tool_result` — sem campo novo.

## Critérios de aceitação

- Edit que introduz `undefined: Foo` → resultado do tool contém a linha do `go vet`; o modelo corrige no passo seguinte (verificar no teste de loop do agente).
- Edit em arquivo não-Go não roda vet.
- Edit fora de módulo Go não roda vet.
- Ambiente sem `go` no PATH → tool funciona normalmente sem diagnóstico.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: temp dir com `go.mod` + arquivo com erro → hook retorna diagnóstico com o nome do símbolo; arquivo válido → "Diagnostics: clean"; sem go.mod → string vazia; teste de agente ponta a ponta com mock: primeira resposta força um `edit` que quebra, o resultado contém o diagnóstico, e a segunda resposta do mock (que "corrigiu") fecha o loop.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Best-effort: nenhum erro de diagnóstico pode falhar o tool em si.
- Sem dependências externas; sem LSP — `go vet` puro.
