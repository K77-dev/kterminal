# Tarefa 1.0: `ExpandMentions` — expansão de menções como função pura

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Expansão de menções
- REQ-003 — Proteções de expansão

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Núcleo isolado e testável da feature: o novo arquivo `internal/tui/mentions.go` expõe `ExpandMentions(text string) (expanded string, warnings []string)` — função pura, testável sem Bubble Tea. Tokens `@<caminho>` válidos são expandidos em blocos `--- Arquivo @caminho ---` anexados ao texto original (que mantém os tokens intactos); arquivo inexistente permanece como texto (o agente decide); máximo de 5 arquivos por mensagem com warning; menções dentro de code fences (4 espaços ou ```) são ignoradas; leitura limitada a 64k por arquivo (mesmo valor do tool `read`, constante duplicada na TUI para evitar acoplamento). Também define `mentionPrefix(input)` para extrair o token em digitação — consumido pelo popup da task 3.0.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`regexp`, `os`, `path/filepath`, `sort`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; função pura determinística (cwd no momento do envio).
</skills>

<requirements>
- Constantes: `maxMentionFiles = 5`, `maxMentionBytes = 64 * 1024` (valor idêntico ao `maxReadBytes` do tool `read`), `maxMentionSuggestions = 10`.
- Regex `mentionRe = @([^\s@]+)` com boundary — `@` precedido de espaço/início (ex.: `email@host` não é menção).
- `ExpandMentions(text) (string, []string)`: percorre linha a linha rastreando code fences (linha iniciada por 4 espaços ou ``` alterna o modo); fora de fences, cada match de arquivo existente e dentro do limite de 5 → lê conteúdo (trunca em 64k + `... (truncated)`); inexistente → token permanece.
- Saída: `<texto original com tokens intactos>` + `\n\n` + blocos `--- Arquivo @caminho ---\n<conteúdo>\n` na ordem de aparição.
- Excedentes além de 5: ficam como texto; `warnings` recebe `"max 5 file mentions"`.
- `mentionPrefix(input) (prefix string, ok bool)`: extrai o token `@...` em digitação (do último `@` sem espaço até o cursor); `@` seguido de espaço não é token.
- Falha de leitura (permissão) trata o arquivo como inexistente — sem erro.
</requirements>

## Subtarefas

- [ ] 1.1 Criar `internal/tui/mentions.go` com constantes, regex e `ExpandMentions`
- [ ] 1.2 Implementar o rastreamento de code fences e o limite de 5 arquivos com warnings
- [ ] 1.3 Implementar `mentionPrefix` (extração do token sob digitação, com boundary)
- [ ] 1.4 Escrever `internal/tui/mentions_test.go` com os 7 testes da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinaturas, constantes e regras de `ExpandMentions`: techspec, seção **Design de Implementação → Interfaces Principais** (subseção "Regras de `ExpandMentions`").
- Decisão "constante 64k duplicada na TUI" (evita acoplamento TUI→tools): techspec, seção **Considerações Técnicas → Decisões Principais** (item 5).
- Boundary do `@` no meio de palavra: techspec, seção **Considerações Técnicas → Riscos Conhecidos**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Os 7 testes da função pura passam (expansão, passthrough de inexistente, múltiplas menções, limite 5, code fences, truncamento 64k, extração de prefixo).
- Nenhuma mudança em `internal/agent` ou `internal/session` (a expansão é transparente para tudo abaixo da TUI).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/mentions_test.go`, função pura com temp dir como cwd — techspec **Abordagem de Testes → Testes Unidade**, itens 1-7):
  - `TestExpandExistingFile` — `@a.txt pergunta` → bloco `--- Arquivo @a.txt ---` + conteúdo; texto original preservado.
  - `TestExpandMissingFilePassesThrough` — `@nope.txt` → saída idêntica à entrada.
  - `TestExpandMultipleMentions` — 3 menções → 3 blocos na ordem.
  - `TestExpandLimitFive` — 6 menções válidas → 5 blocos + warning `"max 5 file mentions"`.
  - `TestExpandIgnoresCodeFences` — menção em linha com 4 espaços ou dentro de ``` → não expandida.
  - `TestExpandTruncatesLargeFile` — arquivo de 100k → truncado em 64k + `... (truncated)`.
  - `TestMentionPrefixExtraction` — `"@int"` → `int`; `"texto @internal/t"` → `internal/t`; sem `@` → ok=false; `@` + espaço → não é token.
- [ ] Testes de integração — fora do escopo da task; o contrato TUI→agent é validado na task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/mentions.go` — novo: `ExpandMentions`, `mentionPrefix`, constantes de limite
- `internal/tui/mentions_test.go` — novo: 7 testes da função pura
- `internal/tools/fs.go` — sem mudança (referência do valor 64k de `maxReadBytes`)
