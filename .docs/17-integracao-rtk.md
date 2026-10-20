# 17 — Integração com rtk (Rust Token Killer)

## Contexto

O kterminal executa comandos de shell via tool `bash` e devolve o stdout/stderr ao LLM. Outputs de comandos como `go test ./...`, `git diff` ou `kubectl describe` são verbosos e consomem muitos tokens — 60–90% do custo de uma sessão agêntica vem de tool results, não de raciocínio.

O **rtk** (Rust Token Killer, https://github.com/rtk-ai/rtk) é um CLI proxy que reescreve comandos shell (`git status` → `rtk git status`) para comprimir a saída, reduzindo o consumo de tokens do LLM. kterminal não está entre os 18 agentes suportados pelo rtk, mas a integração é simples: reescrever o comando via `rtk rewrite "<cmd>"` antes de executar.

Arquitetura relevante:

- `internal/tools/bash.go` — tool `bash` com `runBash`; `run_in_background` desvia para `procmon`.
- `internal/config/config.go` — config TOML com seções `[llm]`, `[router]`, `[squad]`, `[typesafe]`.
- `internal/tui/commands.go` — comandos slash nativos (`/router`, `/model`, `/bg`).
- `main.go` — wiring de config → registry → agent → TUI; `--doctor`.

## Objetivo

Quando o rtk está instalado e habilitado, toda chamada da tool `bash` passa por `rtk rewrite` antes da execução, comprimindo o output sem mudar a semântica do comando. Detecção automática do binário, toggle em runtime via `/rtk`, e diagnóstico no `--doctor`.

## Especificação

1. **Pacote `internal/rtk/`** — `Detector` (cacheia `exec.LookPath("rtk")` thread-safe) e `Rewrite(ctx, command)`.
   - `Rewrite` só reescreve comandos simples: pula comandos vazios, já prefixados com `rtk`, ou com metacaracteres de shell (`&|;<>` backtick `$(` `${` `<<` `>>`) — nesses casos o comando vai direto ao shell.
   - Chamada: `rtk rewrite "<command>"` com timeout de 3s. Se o rtk devolver vazio ou o mesmo comando, o original é preservado.
2. **Config `[rtk]`** — seção TOML com `enabled *bool` (nil = true, mesmo padrão tri-state do `[typesafe] enabled` e `[router] enabled`).
   - Env override: `KTERMINAL_RTK_ENABLED`.
   - `Save()` materializa `enabled = true` quando nil (precedente do `[typesafe]`).
3. **Tool `bash`** — `rtkBashTool` envolve `bashTool`: se o detector está disponível, aplica `applyRtkRewrite` antes de chamar o executor original. O rewrite é transparente: o comando reescrito é o que vai ao shell, o output volta intacto.
4. **Propagação para subagentes** — `Agent.SetRtkDetector` injeta o detector; `newSubagent` propaga ao registry do subagente. Personas do squad e subagentes de execução herdam o rtk automaticamente.
5. **Comando `/rtk`** — `/rtk` ou `/rtk status` mostra on/off; `/rtk on` / `/rtk off` toggle e persistem na config (avisa para reiniciar).
6. **`--doctor`** — linha `rtk enabled: yes/no`; quando habilitado, `rtk binary: found` ou `rtk binary: not found (install with: brew install rtk)`.

## Critérios de aceitação

- Com rtk instalado e habilitado, `git status` executa como `rtk git status` (output comprimido).
- Com rtk desabilitado (`/rtk off`), o comando original executa sem rewrite.
- Comandos com pipes/redirecionamento (`git diff | head`, `echo > file`) não são reescritos — vão direto ao shell.
- Comando já prefixado (`rtk git status`) não é reescrito de novo.
- `rtk rewrite` com timeout de 3s não bloqueia o agente se o rtk travar.
- Subagentes (personas e executores) herdam o detector.
- `/rtk on|off` persiste na config e avisa para reiniciar.
- `--doctor` mostra o estado do rtk e a presença do binário.
- Detecção ausente (rtk não instalado) é silenciosa — o agente funciona normalmente sem rewrite.

## Verificação

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./...
go test ./internal/rtk/ -v
go test ./internal/config/ -run TestLoadRtk -v
go test ./internal/config/ -run TestSaveRtk -v
```

## Restrições

- O rewrite só aplica a comandos simples sem metacaracteres — pipes, redirecionamento, substitution e heredocs vão direto ao shell.
- O rtk não é dependência obrigatória: se o binário não está no PATH, o agente funciona normalmente.
- O rewrite é best-effort: se `rtk rewrite` falhar (erro, timeout, output vazio), o comando original é executado.
- Sem comentários no código; detector thread-safe com cache; `go:embed` não aplicável.
