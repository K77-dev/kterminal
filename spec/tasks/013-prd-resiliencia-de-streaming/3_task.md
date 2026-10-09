# Tarefa 3.0: llm — retry com backoff para falhas transientes

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Retry com backoff para falhas transientes
- REQ-005 — Erros acionáveis na TUI (mensagens finais amigáveis)

## Dependências

- 2.0 (mesmo arquivo, constrói sobre sentinels e estrutura modificada do `ChatStream`)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Loop de tentativas dentro de `ChatStream`: erros transientes (rede, EOF pré-`[DONE]`, 429/5xx, timeouts das camadas) são retentados com backoff exponencial + jitter **apenas enquanto nenhum delta foi emitido ao usuário**; `Retry-After` prevalece quando presente. Esgotadas as tentativas, erro único com mensagem amigável (modelo, tentativas, causa).

<skills>
### Conformidade com Skills Padrões

Padrões do código-base Go (AGENTS.md): sem comentários, stdlib only (`math/rand` para jitter), testes httptest.
</skills>

<requirements>
- Contador `emitted` no `ChatStream`; retry somente com `emitted == 0` e erro transiente
- Transiente: `net.Error`, `io.EOF`/`io.ErrUnexpectedEOF` pré-`[DONE]`, HTTP 429/500/502/503/504, `ErrStreamIdle`, `ErrFirstByte`, deadline da capa total
- Não-transiente: 4xx exceto 429, erros de decode de chunk → retorno imediato
- Erro de status estruturado (`statusError` interno com código e `Retry-After` parseado em delta-seconds)
- Backoff: base 1s, fator 2, capa 30s, jitter ±20%; `Retry-After` (capa 30s) prevalece quando maior
- Erro final: `chat stream failed after N attempts (model X): <causa>` com causa amigável para os sentinels
- `onDelta` nunca invocado mais de uma vez pelo mesmo conteúdo (sem duplicação na TUI)
</requirements>

## Subtarefas

- [ ] 3.1 Criar `statusError` com código + `Retry-After`; substituir o `fmt.Errorf` de não-200 em `do()`
- [ ] 3.2 Implementar classificação `isTransient` cobrindo a lista acima
- [ ] 3.3 Envolver `do()` + leitura em loop de tentativas com backoff/jitter e guarda de `emitted`
- [ ] 3.4 Mensagens finais amigáveis mapeando sentinels e exaustão de tentativas
- [ ] 3.5 Testes: 429 com `Retry-After: 1` → sucesso na 2ª tentativa com intervalo ≥1s; 500 pré-delta com `maxRetries: 1` → 2 tentativas; 500 pós-delta emitido → 1 tentativa e sem duplicar `onDelta`; 401 → sem retry; exaustão → erro com contagem no texto

## Detalhes de Implementação

Ver techspec.md — seções "Retry (em `ChatStream`)" e "Erro estruturado de status". Nos testes, usar `Limits` com `MaxRetries` explícito e backoff curto (a base de 1s pode ser injetável via campo interno não exportado para manter os testes rápidos — ex: `retryBase` no `Client`).

## Critérios de Sucesso

- Falha transiente pré-delta é transparente para o usuário quando o retry sucede
- Falha pós-delta aborta o turno como hoje, sem texto duplicado
- `Retry-After` é honrado com capa de 30s
- `go test ./internal/llm/` verde

## Testes da Tarefa

- [ ] Testes de unidade: classificação transiente (tabela de casos) + cenários httptest (subtarefa 3.5)
- [ ] Testes de integração: N/A
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/llm/llm.go`
- `internal/llm/llm_test.go`
