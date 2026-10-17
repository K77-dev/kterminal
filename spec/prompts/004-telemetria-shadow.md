# Telemetria Shadow (Jev ↔ Laya)

## Descrição

Modo de comparação entre engines de roteamento: quando habilitado, o kterminal roda a decisão de modelo em **ambos** os engines (o ativo + o inativo em "shadow") e registra concordância, confiança e latência de cada um. É telemetria **passiva** — o engine ativo continua decidindo sozinho; o shadow nunca influencia a rota. Atende à motivação "híbrido/ambos": permite avaliar Jev vs Laya com dados reais de uso antes de qualquer troca de default.

## Requisitos Funcionais

- [RF-01] Opt-in explícito: `/router shadow on|off` + `[router] shadow` na config (bool, default `false`). Só roda quando ambos engines estão disponíveis (laya instalada + jev com api key).
- [RF-02] A cada decisão de rota com shadow on, o engine inativo roda em paralelo com o ativo (mesmo state, mesmos candidates) e o kterminal registra: engine ativo, escolha + confiança + latência de cada engine, e concordância (escolheram o mesmo modelo?).
- [RF-03] Registro via **log estruturado em stdout** (o `console.log` do kterminal), sem relatório TUI dedicado — ex.: `{"event": "shadow_route", "active": "jev", "shadow": "laya", "agree": false, "jev": {"model": "...", "confidence": 0.8, "ms": 120}, "laya": {"model": "...", "confidence": 0.9, "ms": 33}}`.
- [RF-04] **Custo como regra de negócio**: quando o engine inativo é o Jev, o shadow consome chamadas pagas da API Typesafe — o opt-in e a documentação (`/help`, README) deixam isso explícito. Shadow da Laya é local e gratuito.
- [RF-05] Falha do engine shadow nunca afeta a decisão real: erro no shadow é logado e descartado (a rota segue a decisão do engine ativo).
- [RF-06] Shadow não roda em subagentes/personas do squad (evita multiplicar custo do Jev por persona) — apenas no agente principal.

## Dependências

- 001 (Engine de roteamento) — o toggle vive no `/router` e na seção `[router]`.
- 003 (LayaRouter) — precisa de ambos engines implementados para comparar.

## Observações

- Pós-MVP: só faz sentido com 001+002+003 entregues.
- O formato do log deve ser estável o suficiente para análise posterior (jq/grep) — campos fixos, um evento por decisão.
- Fora de escopo: relatório TUI (`/router shadow report`), agregação em arquivo local, decisão automática de engine com base na telemetria, shadow em subagentes.
