# Tarefa 5.0: TUI — comando `/jev on|off|status` com re-wiring sem restart

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Toggle persistente do Jev (`/jev on|off`, estado via `/jev`, persistência na config, re-wiring sem restart)

## Dependências

- 4.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Implementar o comando `/jev` na TUI (commands.go): `/jev` sem argumentos exibe o estado atual (on/off, modelo fixo vigente, default do catálogo); `/jev on` habilita (sem api key → erro amigável, nada persiste); `/jev off` desabilita, persiste, re-wira e abre o picker de modelos criado na task 4.0. Toda alteração segue o caminho único da techspec: mutar `cfg` em memória → `cfg.Save()` → `rebuildAgent()`. Com esta task, o REQ-001 fica completo: usuário com chave Typesafe liga/desliga o Jev em runtime sem editar arquivo e sem restart.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês, receivers por valor na TUI, mensagens no vocabulário existente (`errorBox`, confirmações curtas).
</skills>

<requirements>
- `/jev` sem argumentos: exibe estado — on/off, modelo fixo vigente (`cfg.Model` ou default do catálogo)
- `/jev on`: sem api key configurada → `errorBox` amigável, estado inalterado, nada persiste; com chave → `Enabled = true`, save, rebuild
- `/jev off`: `Enabled = false`, save, rebuild, abre o picker de modelos (REQ-003)
- Toda alteração persiste em `config.toml` e re-wira os routers na sessão corrente, sem restart
- `/jev off` com Jev já desabilitado é idempotente (save + picker, sem erro)
- Fluxo de alteração: mutar `m.cfg` → `m.cfg.Save()` → `m.rebuildAgent()` (caminho único da techspec)
</requirements>

## Subtarefas

- [x] 5.1 Implementar parsing de `/jev`, `/jev on`, `/jev off` no dispatcher de comandos (commands.go), com `/jev` exibindo o estado atual
- [x] 5.2 Implementar `/jev on`: guarda de api key (sem chave → `errorBox`, nada persiste), `Enabled = true` + save + rebuild
- [x] 5.3 Implementar `/jev off`: `Enabled = false` + save + rebuild + `openModelPopup()`
- [x] 5.4 Testes de integração em `commands_test.go`/`tui_test.go`: guarda de api key, persistência, abertura do picker, re-wiring

## Detalhes de Implementação

Consulte techspec.md — seção "Comandos TUI" (tabela `/jev`, `/jev on`, `/jev off`) e "Pontos de Persistência". O picker aberto por `/jev off` é o da task 4.0 (`openModelPopup()`); esc no picker mantém o modo automático (default do catálogo — seção "Experiência do Usuário" do PRD).

## Critérios de Sucesso

- `go test ./internal/tui/` verde
- `/jev off` persiste em `config.toml` e sobrevive restart — critério de aceite REQ-001
- `/jev on` sem api key → mensagem de erro clara, nada persiste — critério de aceite REQ-001
- `/jev off` abre o picker automaticamente — critério de aceite REQ-003/REQ-001
- Após `/jev off`, `rebuildAgent()` instala `FixedRouter` e nenhuma chamada ao Jev ocorre na sessão (assert de integração)
- `/jev` sem argumentos exibe on/off e o modelo fixo vigente

## Testes da Tarefa

- [x] Testes de unidade: N/A (comando de TUI; cobertura via integração)
- [x] Testes de integração (`commands_test.go`/`tui_test.go`):
  - [x] `/jev on` sem api key → `errorBox` e arquivo de config inalterado
  - [x] `/jev off` com api key → `enabled = false` persistido (re-`Load` e conferir) + `modelOpen == true`
  - [x] `/jev off` → routers do agente trocados para `FixedRouter` (nenhuma requisição ao mock do Jev em turno seguinte)
  - [x] `/jev on` com api key → `enabled = true` persistido + `JevRouter` reinstalado
  - [x] `/jev` sem argumentos → bloco de estado com on/off e modelo vigente
- [x] Testes E2E: N/A (verificado via transcript na task 7.0)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/commands.go` (modificado — `/jev`)
- `internal/tui/commands_test.go`, `internal/tui/tui_test.go` (modificados)
- `internal/tui/tui.go` (referência — `rebuildAgent()` da task 4.0, `openModelPopup()` da task 4.0)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência)
