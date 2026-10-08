# Tarefa 1.0: Content parts no `llm.Message` com `MarshalJSON` condicional

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Mensagens com content parts

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Fundação de contrato da feature: `llm.Message` ganha suporte a content parts OpenAI-compatible — campo `ContentParts []ContentPart` com `MarshalJSON` customizado. Sem parts, o request é **byte a byte igual ao atual** (`"content": "string"` — compatibilidade total, REQ-001 duro, travado por golden test); com parts, o content vira array (`text` + `image_url` com data URI base64). O `Content string` forte é mantido — a alternativa `Content any` quebraria todos os callers e a compatibilidade (decisão 1 da techspec). O roundtrip marshal→unmarshal preserva os campos (base para o snapshot da techspec 004).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`encoding/json`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; golden tests para travar contrato.
</skills>

<requirements>
- `ImageURL{URL string}` com `json:"url,omitempty"`.
- `ContentPart{Type, Text, ImageURL}` com tags `text,omitempty` e `image_url,omitempty`.
- `Message.ContentParts []ContentPart` + `MarshalJSON` customizado.
- `MarshalJSON`: `len(ContentParts) == 0` → `{"role": ..., "content": "<string>", ...}` idêntico ao atual; com parts → `"content": [{"type":"text","text":...}, {"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}]`.
- Golden test trava o formato string byte a byte — regressão de contrato impossível de passar despercebida.
- Roundtrip marshal→unmarshal preserva campos (incluindo `ToolCalls`/`ToolCallID` existentes).
</requirements>

## Subtarefas

- [ ] 1.1 Definir `ContentPart` e `ImageURL` em `internal/llm/llm.go`
- [ ] 1.2 Adicionar `ContentParts` ao `Message` e implementar o `MarshalJSON` condicional
- [ ] 1.3 Escrever golden test do formato string + teste de parts + roundtrip (ver Testes da Tarefa)

## Detalhes de Implementação

- Structs e regras do `MarshalJSON`: techspec, seção **Design de Implementação → Interfaces Principais** (subseção "MarshalJSON").
- Decisão "`MarshalJSON` condicional em vez de `Content any`": techspec, seção **Considerações Técnicas → Decisões Principais** (item 1).
- Ponto único de compatibilidade com golden test: techspec, seção **Verificações Técnicas → Arquitetura**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Golden test prova: sem parts → JSON byte a byte igual ao golden atual.
- Teste prova: com parts → array com `text` + `image_url` (data URI) na ordem.
- Teste prova: marshal → unmarshal → campos preservados.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/llm/llm_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 1-3):
  - `TestMessageMarshalStringContentGolden` — sem parts → JSON byte a byte igual ao golden (compatibilidade REQ-001).
  - `TestMessageMarshalContentParts` — com parts → array `text` + `image_url` na ordem.
  - `TestMessageUnmarshalRoundtrip` — marshal → unmarshal → campos preservados (snapshot da techspec 004).
- [ ] Testes de integração — fora do escopo da task; o request ao gateway é validado na task 4.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/llm/llm.go` — `ContentPart`, `ImageURL`, `MarshalJSON` condicional
- `internal/llm/llm_test.go` — golden string content, parts array, roundtrip
