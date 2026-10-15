# 15 — Controle do Jev: toggle de roteamento e modelo fixo

## Contexto

O Jev é habilitado hoje de forma implícita: `main.go:82-90` e `rebuildAgent()` (`internal/tui/tui.go`) instanciam o `JevRouter` somente quando `typesafe.api_key` está presente; sem a chave, o `HeuristicRouter` assume sozinho. Não existe toggle explícito — quem tem a chave não consegue desligar o Jev sem remover a chave da config, e quem não tem (ou não quer) roteamento dinâmico fica nas mãos do `HeuristicRouter` (`internal/router/router.go:74`), que decide por keywords e varia o modelo a cada passo.

O `/model <name>` existe como pin por sessão (`Agent.SetPinned`, `internal/agent/agent.go:145`): contorna o router com `Router: "pin"`, mas não persiste — reiniciar o kterminal perde a escolha. O `/model auto` remove o pin da sessão corrente.

Arquitetura relevante:

- `internal/config/config.go` — TOML `[llm]`, `[typesafe]`, `[squad]`, `[agent]`; sem toggle de roteamento, sem campo de modelo fixo.
- `internal/router/router.go` — interface `Router`, `JevRouter`, `HeuristicRouter`.
- `main.go` e `internal/tui/tui.go` (`rebuildAgent`) — wiring: JevRouter se api key, HeuristicRouter como fallback de erro.
- `internal/tui/commands.go:120-170` — popup do `/mode` como precedente de seletor interativo (setas + enter + esc).
- `internal/agent/agent.go` `decide()` — pin vence; senão router; fallback em erro; subagentes herdam os routers via `SetRouters`.

## Objetivo

Toggle explícito e persistente do Jev — `[typesafe] enabled` na config + comando `/jev on|off` na TUI — e, com o Jev desabilitado, roteamento determinístico por modelo fixo: escolhido via picker interativo ou `/model`, persistido na config, com o `default_model` do catálogo como fallback quando nenhum modelo for definido.

## Especificação

1. **Config**: `[typesafe]` ganha `enabled` (bool, default `true` — retro-compatível: Jev ativo ⇔ `enabled = true` **e** `api_key` presente). Novo campo top-level `model` (string, default `""`) — modelo fixo de roteamento; vazio = automático. Env overrides `KTERMINAL_TYPESAFE_ENABLED` e `KTERMINAL_MODEL`. Modelo inválido na config → erro claro no startup (validação contra o catálogo em `main.go`, não no `config.Load`).

2. **Roteamento com Jev desabilitado**: novo `FixedRouter` (nome final na techspec) que retorna sempre o modelo fixo (`config.model`) ou o `default_model` do catálogo quando vazio — determinístico, sem keywords, `Router: "fixed"`. Subagentes e personas herdam o mesmo wiring (`SetRouters` é global). O `HeuristicRouter` deixa de ser roteador principal; permanece apenas como fallback de erro do Jev quando habilitado (comportamento atual).

3. **Comando `/jev`**: `/jev` exibe o estado (on/off, modelo fixo vigente, default do catálogo); `/jev on` habilita (sem api key → erro amigável); `/jev off` desabilita e abre o picker de modelos. Toda alteração persiste `[typesafe] enabled` via `cfg.Save()` e re-wira os routers via `rebuildAgent()`.

4. **Picker de modelos**: popup no estilo do `/mode`, listando os modelos do catálogo (nome + preço/velocidade resumidos) e uma entrada `auto`; setas navegam, enter seleciona, esc cancela sem alterar. Seleção persiste `model` na config, aplica como pin e re-wira. `/model` sem argumentos abre o mesmo picker; `/model <name>` persiste direto; `/model auto` limpa `model` (Jev volta a decidir se habilitado; senão default do catálogo).

5. **Tela `/config`**: ganha dois campos — `jev enabled (yes/no)` e `model` (nome ou vazio) — salvos junto com os demais em `saveConfig()`.

6. **Doctor, help e sugestões**: `--doctor` imprime `jev enabled` e o modelo fixo efetivos; `/help` documenta `/jev` e o novo `/model`; `nativeCommands` ganha `/jev`.

7. **Status bar**: reflete o roteamento atual — `jev` quando roteando via Jev, `jev off` quando desabilitado, `pinned <model>` quando modelo fixo (indicador existente).

## Critérios de aceitação

- `[typesafe] enabled = false` com api key presente → nenhuma chamada ao Jev; todas as requisições (agente e subagentes) usam o modelo fixo.
- Jev desabilitado + `model` vazio → todas as chamadas usam o `default_model` do catálogo, o mesmo modelo em todos os passos.
- `/jev off` abre o picker; a seleção persiste em `config.toml` e sobrevive restart.
- `/jev on` sem api key → erro amigável; com api key → Jev volta a rotear (modelo fixo definido continua vencendo, como o pin hoje; `/model auto` limpa).
- `/model <name>` persiste na config e sobrevive restart; `/model` sem argumentos abre o picker; `/model auto` limpa.
- Config existente sem `enabled`/`model` → comportamento atual preservado (Jev ativo com api key, heurística sem chave).
- `model` inválido na config → erro claro no startup.
- `--doctor`, `/help` e sugestões de comando refletiem a feature.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: config (defaults, `enabled`/`model`, env overrides, retro-compatibilidade); router fixo determinístico (mesmo modelo sempre, `Router: "fixed"`); agent (`decide` com Jev desabilitado usa modelo fixo; subagente herda; pin continua vencendo); TUI (`/jev` toggle persiste e re-wira, picker navega/seleciona/cancela, `/model` persiste, status bar, `/config` com novos campos); doctor.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Sem mudanças no cliente Jev (`internal/jev`) nem no protocolo Typesafe.
- Squad pins (`[squad] pins`) e telemetria de TPS fora de escopo.
- Eventos do agente via canal; TUI com receivers por valor; `strings.Builder` sempre por ponteiro.
