# LayaRouter

## Descrição

Implementação da interface `Router` do kterminal (`internal/router`) usando o motor Laya embutido (módulo 002) — a Laya passa a exercer **exatamente o mesmo papel do Jev**: escolher o LLM do catálogo a cada chamada do agente, via `Route(ctx, state, candidates) → Decision{Model, Confidence, Probabilities, Router}`. Com este módulo, `/router laya` passa a funcionar de fato: o roteamento dinâmico roda local, sem API externa, ~33ms por decisão.

## Requisitos Funcionais

- [RF-01] `LayaRouter` implementa `Router.Route(ctx, state, candidates)`: monta a **mesma pergunta do Jev** — uma única `choice` (`model_choice`) com `instructions` idênticas às do `JevRouter` (`router.go`) e `criteria = Criteria()` de cada modelo candidato — e devolve `Decision` com `Router: "laya"`.
- [RF-02] Casos degenerados idênticos ao `JevRouter`: zero candidatos → erro; candidato único → decisão direta sem inferência (`Confidence: 1`, reason "single candidate").
- [RF-03] Fallback heurístico: se a inferência Laya falhar (timeout, checkpoint ausente/corrompido, erro de runtime), cai para o `HeuristicRouter` local — mesmo comportamento do Jev hoje (erro do Jev → heurística).
- [RF-04] A escolha da Laya é validada contra os candidatos (escolha fora da lista → erro → fallback heurístico), como o `JevRouter` valida a resposta do Jev.
- [RF-05] Wiring no `router.Wire()` (módulo 001): `engine = "laya"` → `LayaRouter` como roteador principal + `HeuristicRouter` como fallback; subagentes e personas do squad herdam o mesmo wiring via `SetRouters` (global).
- [RF-06] O `state` enviado ao motor segue o mesmo formato do state do Jev (resumo do passo corrente + papel da persona no squad mode + sinalização de anexos de imagem) — sem adaptação especial para a Laya.

## Dependências

- 002 (Motor Laya embutido) — fornece a API `Predict` que este módulo consome.
- 001 (Engine de roteamento) — fornece o mecanismo de seleção/wiring onde o `LayaRouter` pluga.

## Observações

- Módulo deliberadamente fino: toda a complexidade (ONNX, download, tokenização) vive no 002; este módulo só mapeia o contrato `Router` ↔ motor.
- A Laya responde em uma forward pass não-autoregressiva — não há streaming nem parse de texto gerado; a resposta já vem tipada com probabilidades calibradas.
- O pin manual (`/model`) continua vencendo o router, como hoje; `FixedRouter` permanece o roteador quando o roteamento dinâmico está off.
- Fora de escopo: perguntas extras (score/noul) além da choice de modelo, telemetria shadow (004), mudanças no `HeuristicRouter`.
