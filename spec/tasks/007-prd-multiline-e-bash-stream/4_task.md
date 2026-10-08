# Tarefa 4.0: Prompt multi-linha com textarea e histórico de prompts

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Prompt multi-linha
- REQ-002 — Histórico de prompts

## Dependências

- Nenhuma (independente das tasks 1.0-3.0 — é a outra metade do PRD)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

A troca do input: o `textinput` single-line dá lugar ao `bubbles/textarea` (mesma família de dependências já no `go.mod`) com keymap invertido explicitamente — o default do componente é o inverso do desejado (enter quebra linha), por isso a interceptação acontece em `handleChatKey` **antes** de delegar ao textarea: `enter` envia, `shift+enter` quebra linha via `InsertString("\n")`. Altura automática de 1 a 8 linhas conforme o conteúdo (linhas lógicas, não visuais); viewport nunca menor que 3 linhas. Histórico dos últimos 20 prompts em memória: ↑/↓ com input vazio navegam; digitar qualquer coisa reseta a navegação. O contrato com o agent não muda — `Run(texto)` recebe `\n`.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — `bubbles/textarea` já no `go.mod` (v1.0.0); nenhuma dependência nova.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (multiline, histórico), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; `strings.Builder` por ponteiro; estilo visual do cursor/texto preservado (fundo/caixa iguais ao atual).
</skills>

<requirements>
- `input` troca de `textinput.Model` para `textarea.Model` com `Prompt = ""`, `ShowLineNumbers = false`, `SetMaxWidth`, `TextStyle` com `Background(colBgElement)` — visual idêntico ao atual.
- Interceptação em `handleChatKey` antes de delegar: `tea.KeyEnter` sem shift → envia; `shift+enter` → `InsertString("\n")` e consumir a tecla.
- Altura: após cada mudança de conteúdo, `SetHeight(clamp(1, linhas, 8))`; viewport = altura disponível com mínimo 3 (`minViewportRows` extraída para reuso).
- Altura calculada por linhas lógicas (`\n`), não visuais.
- `promptHistory []string` (máx. 20, FIFO) + `histIdx`: ↑ com input vazio carrega o anterior; ↓ avança até voltar ao vazio; digitar reseta a navegação.
- Envio preserva as quebras de linha (`\n` chega intacto ao `agent.Run`).
- Histórico em memória apenas — não persiste entre sessões (fora de escopo do PRD).
</requirements>

## Subtarefas

- [ ] 4.1 Trocar `textinput` por `textarea` com visual idêntico ao atual
- [ ] 4.2 Interceptação de teclas: enter envia, shift+enter quebra linha (antes de delegar ao textarea)
- [ ] 4.3 Altura automática 1-8 e viewport mínimo 3 (extrair `minViewportRows`)
- [ ] 4.4 Implementar histórico de 20 prompts com ↑/↓ e reset ao digitar
- [ ] 4.5 Escrever os testes 8-11 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Configuração do textarea, keymap e alturas: techspec, seção **Design de Implementação** (subseção "Multi-linha na TUI").
- Decisão "interceptação de teclas na TUI em vez de remontar o keymap": techspec, seção **Considerações Técnicas → Decisões Principais** (item 1).
- Decisão "histórico em memória (20)": techspec, seção **Considerações Técnicas → Decisões Principais** (item 4).
- Riscos (largura visual × lógica, paste multi-linha): techspec, seção **Considerações Técnicas → Riscos Conhecidos**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: `shift+enter` → input contém `\n`; altura do prompt = 2.
- Teste prova: input com 2 linhas + `enter` → `agent.Run` recebe o texto com `\n` preservado.
- Teste prova: 12 linhas digitadas → altura do prompt = 8; viewport ≥ 3.
- Teste prova: enviar 3 prompts → ↑ carrega o 3º e depois o 2º; ↓ volta; digitar cancela; 21º descarta o mais antigo.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 8-11):
  - `TestShiftEnterCreatesNewline` — `\n` no input; altura 2.
  - `TestEnterSendsMultiline` — envio com `\n` preservado ao agent.
  - `TestPromptHeightClamped` — 12 linhas → altura 8; viewport ≥ 3.
  - `TestHistoryNavigation` — ↑/↓ navegam; digitar reseta; FIFO de 20.
- [ ] Testes de integração — o contrato com o agent é validado por `TestEnterSendsMultiline` (captura do input).
- [ ] Testes E2E — deferidos para `kspec-qa` (`shift+enter` em prompt longo; ↑ recupera prompt anterior).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — textarea multi-linha, keymap, alturas, histórico
- `internal/tui/tui_test.go` — shift+enter, envio multi-linha, clamp de altura, histórico
- `go.mod` — sem mudança (`bubbles/textarea` já é dependência)
