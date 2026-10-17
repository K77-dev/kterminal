# Engine de Roteamento (Jev ↔ Laya)

## Descrição

O kterminal é um coding agent de terminal em Go (Bubble Tea) cujo diferencial é o roteamento dinâmico de LLM: o **Jev** (roteador externo da Typesafe, API `api.typesafe.ai/v1/systemone`) escolhe o modelo do catálogo a cada chamada, via `internal/router` (`JevRouter` com fallback `HeuristicRouter`, `FixedRouter` quando desligado). Hoje o controle é o comando `/jev on|off` + `[typesafe] enabled` na config.

Este módulo introduz o conceito de **engine de roteamento selecionável**: o usuário escolhe entre **Jev** (API externa) e **Laya** (decision engine local, https://github.com/NandhaKishorM/laya — que fará o mesmo papel do Jev: decidir o modelo a cada chamada). A Laya não depende de api key (roda local); o Jev continua exigindo `typesafe.api_key`. Este módulo entrega o mecanismo de seleção — config, comando TUI, status, doctor — abstraindo o wiring para qualquer engine.

## Requisitos Funcionais

- [RF-01] Nova seção `[router]` na config com campo `engine` (`"jev"` | `"laya"`, default `"jev"`). Retrocompatível: config existente sem `[router]` → `engine = "jev"` e comportamento atual preservado.
- [RF-02] `engine = "laya"` funciona **sem** api key do Typesafe — a Laya é independente do `[typesafe]`. `engine = "jev"` sem api key → erro amigável (como `/jev on` sem chave hoje).
- [RF-03] Comando `/router` **substitui** o `/jev`: `/router` ou `/router status` exibe o estado (engine ativo, on/off, modelo fixo vigente); `/router jev` e `/router laya` trocam o engine; `/router off` desliga o roteamento dinâmico (fixed, abre picker de modelos como hoje); `/router on` religa com o engine ativo.
- [RF-04] Toda alteração persiste na config via `cfg.Save()` e re-wira os routers via `rebuildAgent()` sem restart — mesmo padrão do `/jev` atual.
- [RF-05] Status bar reflete o engine ativo: `jev` ou `laya` quando roteando, `jev off`/`laya off` quando desligado, `pinned <model>` quando fixo.
- [RF-06] `--doctor` informa o engine efetivo; `/help` documenta `/router`; `nativeCommands` e sugestões trocam `/jev` por `/router`; tela `/config` expõe o campo engine.
- [RF-07] Env override `KTERMINAL_ROUTER` (padrão dos overrides existentes, ex.: `KTERMINAL_TYPESAFE_ENABLED`).

## Dependências

- Nenhuma — módulo fundação. Os engines Jev (existente) e Laya (módulos 002/003) plugam nele.

## Observações

- Enquanto o módulo 003 (LayaRouter) não existir, `/router laya` deve falhar com erro claro ("laya engine ainda não disponível") e **não** mudar a config — o mecanismo de seleção já fica pronto e testado.
- O fallback de erro de roteamento continua sendo o `HeuristicRouter` local, independente do engine.
- `/model` (pin por sessão + modelo fixo persistido) não muda — é ortogonal ao engine.
- Fora de escopo: o motor de inferência da Laya (002), o `LayaRouter` (003), telemetria shadow (004), mudanças no cliente Jev (`internal/jev`) ou no protocolo Typesafe.
