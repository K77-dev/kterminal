# Tarefa 2.0: llm — timeouts de streaming em camadas

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Timeouts de streaming em camadas

## Dependências

- Nenhuma (paralela com 1.0 e 4.0)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Substituir o deadline único de 10 min por três camadas independentes dentro de `internal/llm`: primeiro byte (timer que cancela o ctx até os headers chegarem), idle do stream (watchdog sobre `resp.Body` resetado a cada leitura) e capa total (`http.Client.Timeout` configurável). Exportar `Limits`, `SetLimits` e os sentinels `ErrStreamIdle`/`ErrFirstByte`.

<skills>
### Conformidade com Skills Padrões

Padrões do código-base Go (AGENTS.md): sem comentários, stdlib only, testes com `httptest` e `t.Context()` seguindo `llm_test.go`.
</skills>

<requirements>
- `Limits{RequestTimeout, IdleTimeout, FirstByteTimeout time.Duration; MaxRetries int}` com defaults 10m/5m/60s/2 aplicados em `New`
- `New` mantém a assinatura atual; `SetLimits(Limits)` sobrescreve
- `RequestTimeout == 0` → sem capa total (`http.Client.Timeout = 0`)
- First byte: `time.AfterFunc` cancela ctx derivado se `Do` não retornar; timer parado no retorno; cancel só após consumo do body
- Idle: wrapper do `resp.Body` reseta timer por `Read`; expirou → cancela ctx do request e retorna erro wrapped em `ErrStreamIdle`; sem goroutine/timer vazando
- `ChatStream` lê através do wrapper; parsing SSE inalterado
- Erros de first-byte identificáveis via `errors.Is(err, ErrFirstByte)`
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `Limits`, campos no `Client`, `SetLimits`; mover o 10m fixo para default de `Limits.RequestTimeout`
- [ ] 2.2 Implementar timer de primeiro byte em `do()` com cancelamento adiado
- [ ] 2.3 Implementar `idleBody` (watchdog de idle) e envolver o body em `ChatStream`/`do`
- [ ] 2.4 Testes com `Limits` curtos (ex: 100ms/150ms): stall pré-header → `ErrFirstByte`; headers + 2 chunks e trava → `ErrStreamIdle` na janela; stream saudável com chunks espaçados menores que idle completa; capa total continua funcionando (`RequestTimeout` curto aborta stream lento)

## Detalhes de Implementação

Ver techspec.md — seção "Mecanismos de timeout (em `llm`)". Atenção ao detalhe crítico: cancelar o contexto do request aborta a leitura do body, então o cancel do primeiro byte só pode rodar depois que o body foi consumido/fechado (defer após o stream terminar). O watchdog de idle cancela o mesmo contexto para desbloquear um `Read` travado — distinguir a causa com flag/marker antes de cancelar.

## Critérios de Sucesso

- Stream travado após chunks é abortado dentro da janela de idle com `ErrStreamIdle` (não espera a capa total)
- Gateway que não entrega headers falha com `ErrFirstByte` na janela de primeiro byte
- Stream saudável mais longo que 10m completa com `SetLimits(Limits{RequestTimeout: 0, ...})`
- `go test ./internal/llm/` verde, inclusive com `-race`

## Testes da Tarefa

- [ ] Testes de unidade: stalls com httptest e canais de controle (subtarefa 2.4)
- [ ] Testes de integração: N/A (o httptest já cobre o caminho HTTP real)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/llm/llm.go`
- `internal/llm/llm_test.go`
