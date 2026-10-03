# 06 — Telemetria real de TPS alimentando o catálogo

## Contexto

kterminal é um terminal agêntico em Go onde o Jev (Typesafe) escolhe o LLM a cada chamada, pesando qualidade, custo e tokens/s. Hoje o TPS que o Jev vê vem de `tps_estimate` — estimativas estáticas no YAML. O pitch do produto é "melhor TPS"; medir o TPS real de uso e alimentar a decisão do Jev fecha esse ciclo com dados próprios.

Arquitetura relevante:

- `internal/agent/agent.go` — após cada `ChatStream`, já computa `tps = CompletionTokens / StreamSeconds`.
- `internal/catalog/catalog.go` — `Model.TPSEstimate` e `Model.Criteria()` (o texto que o Jev vê por modelo).
- `internal/session/session.go` — padrão de persistência em `~/.local/share/kterminal/`.

## Objetivo

Medir TPS real por modelo no uso, persistir estatísticas rolling, e fazer o `Criteria()` usar TPS medido quando houver amostras suficientes — o Jev decide com dados reais.

## Especificação

1. Novo pacote `internal/telemetry`:
   - `Store{Models map[string]Stats}` com `Stats{Samples int, Mean float64, M2 float64}` (média rolling via algoritmo de Welford, sem guardar histórico).
   - `Record(model string, tps float64)` — descarta outliers grosseiros (tps <= 0 ou > 10000).
   - `Get(model) (mean float64, n int)`.
   - Persistência em `~/.local/share/kterminal/telemetry.json` (load no início, save a cada gravação com debounce — salvar no máximo 1× a cada 10s e no encerramento).
   - Janela rolling: manter também um EWMA com α=0.3 por modelo; `Get` retorna o EWMA quando `Samples >= 5`, senão zero.
2. `Agent` grava `telemetry.Record(model, tps)` após cada chamada com tokens de output > 0.
3. `catalog.Model` ganha campo calculado `MeasuredTPS float64` (não vem do YAML). O agent, ao montar candidatos para o router, injeta o valor medido em cada modelo.
4. `Model.Criteria()`: quando `MeasuredTPS > 0`, usa `Speed: ~X tok/s (measured over N calls)`; senão, mantém o estimate atual.
5. `kterminal --doctor` imprime a tabela de TPS medido por modelo (mean, samples) ao lado dos estimates.
6. TUI: nada muda visualmente (o TPS exibido no hint bar já é o medido por resposta).

## Critérios de aceitação

- Após 5+ chamadas num modelo, o `Criteria()` enviado ao Jev cita TPS medido.
- Reiniciar o app não perde as estatísticas (persistência funciona).
- Amostras absurdas (0, negativas) não contaminam a média.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: Welford/EWMA (média correta com sequência conhecida, outlier descartado, EWMA converge); persistência roundtrip em temp dir; `Criteria()` alterna entre estimate e measured conforme `Samples`.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Sem dependências externas; stats implementados à mão.
- O YAML continua sendo o fallback — nunca piorar a informação do Jev.
