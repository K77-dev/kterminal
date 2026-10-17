# Escolha de Engine de Roteamento (Jev ↔ Laya) — Plano de Módulos

## Visão Geral

O kterminal hoje roteia LLMs via **Jev** (roteador externo da Typesafe). Esta feature introduz a **Laya** (https://github.com/NandhaKishorM/laya — decision engine System 1 local, não-autoregressivo, ~33ms) como alternativa que exerce **exatamente o mesmo papel**: escolher o modelo do catálogo a cada chamada. O usuário escolhe o engine via config/comando TUI; a Laya roda **embutida no binário** (inferência ONNX in-process, sem servidor externo); e um modo shadow opt-in registra comparações entre engines para avaliar Jev vs Laya com dados reais.

## Módulos

| # | Módulo | Descrição | MVP | Dependências |
|---|--------|-----------|-----|--------------|
| 001 | Engine de Roteamento | Seleção de engine via `[router] engine` + `/router` (substitui `/jev`), status bar, doctor, wiring | Sim | — |
| 002 | Motor Laya Embutido | Inferência Laya in-process no binário Go: ONNX Runtime Go, 3 checkpoints, download no 1º uso com progresso, cache em `~/.config/kterminal/laya` | Sim | — |
| 003 | LayaRouter | `Router` interface sobre o motor Laya: mesma pergunta do Jev, fallback heurístico, wiring no `router.Wire()` | Sim | 001, 002 |
| 004 | Telemetria Shadow | Opt-in, roda o engine inativo em paralelo e loga concordância/confiança/latência em stdout | Não | 001, 003 |

## MVP

O MVP (001+002+003) entrega a escolha real entre Jev e Laya: o usuário seleciona o engine via `/router` ou config, a Laya decide o modelo a cada chamada rodando local (sem api key, sem servidor), com fallback heurístico na falha — paridade completa de papel com o Jev. O módulo 004 (shadow) vem depois, como telemetria passiva de comparação.

## Como Usar

Para gerar o PRD de cada módulo, execute:

`/kspec-prd` passando o conteúdo do arquivo de prompt correspondente.

Recomenda-se seguir a ordem numérica dos módulos.
