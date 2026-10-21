# 16 — Plan × Build (camada de permissão com tab/alt+tab)

## Contexto

kterminal tem dois modos de trabalho (`/mode`): `sdd` (fluxo de skills) e `squad` (mesa de personas). Falta o par **plan/build** do opencode: uma camada de permissão independente que deixa o agente pesquisar e planejar sem risco de execução — o usuário revisa o plano antes de liberar a implementação.

Arquitetura relevante:

- `internal/agent/agent.go` — `squadBlocksTool()` já é o precedente de bloqueio mecânico de tools mutantes no maestro; `toolDefinitions()` filtra o que é oferecido ao modelo; `writeSnapshot()` persiste `mode` no snapshot.
- `internal/agent/prompt.go` — `rebuildSystemPrompt()` compõe base + rules + skill + addenda (`maestroAddendum`).
- `internal/tui/tui.go` — `handleChatKey()` roteia teclas do estado chat; popups (menção/comando/modo/modelo/bg) interceptam `tab` antes.
- `internal/tui/commands.go` — `/mode` e o popup de modos; `hintBar()` em `tui.go` mostra o estado da sessão.
- `internal/session/session.go` — `Snapshot` carrega `Skill`/`Mode`/`Mesa`; `main.go` restaura no resume.

## Objetivo

Alternância instantânea entre **plan** (pesquisa e planejamento apenas) e **build** (toolset completo) com `tab` (input vazio) e `alt+tab`, no estilo do opencode — sem popup, com anúncio no chat e indicador no hint bar.

## Especificação

1. **Camada separada de modo**: `permission` (`build`|`plan`) no `Agent`, independente de `mode` (`sdd`|`squad`). `SetPermission`/`Permission`/`TogglePermission`; inválido ou mid-turn retorna erro.
2. **Bloqueio em plan**: `planBlocksTool()` bloqueia todo tool mutante (`write`, `edit`, `bash`, `kspec_bootstrap`) nos dois modos. `task` é bloqueado em sdd; na mesa ativa (`maestroActive()`) ele permanece para convocações de personas (registry read-only), e o execution handoff (`task` sem persona) é recusado dentro do próprio tool em plan. `toolDefinitions()` deixa de oferecer os bloqueados; `executeTool()` devolve erro instrutivo se o modelo tentar. Livres: `read`, `glob`, `grep`, `ask_user`, `squad_kickoff`.
3. **Uniformidade entre modos**: a camada plan/build é a única fonte de permissão de escrita, válida em `sdd` e `squad`. O squad não tem mais bloqueio mecânico próprio do maestro (`squadBlocksTool` removido): em build o maestro carrega o toolset completo e pode executar diretamente; em plan pesquisa e delibera apenas.
4. **Toggle**: `tab` com input vazio (sem popup aberto) e `alt+tab` a qualquer momento chamam `TogglePermission()`, nos dois modos. Mid-turn também é bloqueado (`errTurnActive`).
5. **Prompt**: `planAddendum` no system prompt em plan (composto também sobre o maestro prompt em squad) — instrui pesquisar, perguntar e entregar plano completo.
6. **UI**: anúncio no chat ao alternar (`◐ plan mode …` / `◆ build mode …`); hint bar ganha segmento `plan` (amber) quando ativo; `/help` documenta o atalho.
7. **Persistência**: campo `permission` (aditivo, `omitempty`) em `session.Event`/`Snapshot`; `WriteSnapshot` o carrega; `main.go` restaura no resume. Snapshots legados sem o campo voltam como `build`.

## Critérios de aceitação

- `tab` com input vazio alterna build ↔ plan; `alt+tab` alterna mesmo com texto no input (input intacto) — nos dois modos.
- Em plan, o modelo não recebe `write`/`edit`/`bash`/`kspec_bootstrap` nas tool definitions; chamada direta retorna erro "plan mode". Em sdd, `task` também é bloqueado; na mesa, `task` fica para convocações e o handoff de execução retorna erro "plan mode".
- `read`/`glob`/`grep` continuam disponíveis em plan.
- Em squad build, o maestro carrega o toolset completo (escrita liberada); em squad plan, pesquisa e delibera apenas.
- Hint bar mostra `plan` apenas em plan; anúncio aparece no chat a cada toggle.
- Snapshot carrega a permissão; resume a restaura; snapshot legado resume como build.
- Toggle mid-turn falha sem efeito colateral.

## Verificação

```bash
go build ./... && go vet ./...
go test ./internal/agent/ -run 'TestPlan|TestTogglePermission' -v
go test ./internal/tui/ -run 'TestTab|TestAltTab|TestPlanMode|TestResumeRestoresPlanPermission' -v
go test ./internal/session/ -run 'TestSnapshotCarriesPermission|TestSnapshotOmitsEmptyPermission' -v
go test ./...
```

## Restrições

- Sem popup para o toggle — troca direta com feedback no chat (comportamento opencode).
- `tab` continua completando menção/comando quando esses popups estão abertos; o toggle só roda quando nenhum popup está aberto e o input está vazio.
- Não introduzir novo comando slash — o par plan/build vive no atalho.
- Camada plan não pode quebrar o squad mode (mesa preservada, `task` liberado para convocações do maestro; apenas o handoff de execução é recusado).
