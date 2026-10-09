# Tarefa 4.0: TUI — render do sidebar (`sidebar.go` + estilos): largura, mesa, maestro, personas, rodapé de orçamento, elisão

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Painel da mesa (lista de personas, maestro no topo, mesa não iniciada)
- REQ-003 — Atividade ao vivo (linha de atividade truncada com elisão)
- REQ-004 — Métricas por persona e orçamento da mesa (modelo/tokens/custo, rodapé com tetos)
- REQ-005 — Layout responsivo (piso/teto de largura, clamp proporcional)

## Dependências

- 3.0 (getters `Mesa()`/`RestoreMesa()` do Agent para injetar a Mesa nos testes)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-6h

## Visão Geral

Criar `internal/tui/sidebar.go` com o render puro do painel: `sidebarWidth() int` (0 quando oculto) e `sidebarView(width int) string`. O painel desenha cabeçalho, linha do maestro (identificação, status, modelo — sem métricas próprias), entradas por persona (nome + status textual + cor da disciplina, modelo, tokens, custo), rodapé de orçamento (`n/max` convocações e `k/budget` tokens) e o estado "mesa não iniciada" quando a Mesa é nil. Nesta task o sidebar ainda não é composto na `View()` — é o render isolado, testável com Mesa injetada via `RestoreMesa`.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, receivers por valor em render, `strings.Builder` por ponteiro, textos da TUI em inglês, truncar por runes com `truncate` existente, medir com `lipgloss.Width` nos testes.
</skills>

<requirements>
- `sidebarWidth()`: 0 quando oculto (modo sdd ou terminal < 100 cols); proporcional à largura do terminal com piso 24 e teto 40 (100→24, 140→35, 200→40)
- `sidebarView(width int) string`: cabeçalho, maestro no topo, entradas na ordem de convocação, rodapé de orçamento
- Nome e status sempre textuais; cor da disciplina (paleta existente `disciplineColor`) nunca é o único indicador
- Conteúdo maior que a largura do painel é truncado por runes com elisão explícita "…", sem quebrar o layout
- Mesa nil → estado textual explícito de mesa não iniciada, nunca vazio silencioso
- Persona sem atividade → "—"
- Estilos em `theme.go`: borda/gutter (`colBorder`), fundo (`colBgPanel`), reuso de `disciplineColor`
- Sidebar é somente leitura: string renderizada, sem foco, sem handler de tecla
</requirements>

## Subtarefas

- [ ] 4.1 Implementar `sidebarWidth()` com clamp piso/teto e 0 quando oculto
- [ ] 4.2 Implementar `sidebarView(width)`: cabeçalho, linha do maestro, entradas por persona (nome, status, cor da disciplina, modelo, tokens, custo), rodapé `n/max` e `k/budget`
- [ ] 4.3 Estado "mesa não iniciada" com Mesa nil; elisão "…" em conteúdo longo; "—" para persona sem atividade
- [ ] 4.4 Adicionar estilos do sidebar em `theme.go` (borda/gutter, fundo, reuso da paleta por disciplina)
- [ ] 4.5 Testes: clamp de largura, conteúdo com Mesa injetada, elisão, mesa não iniciada, contraste textual (cor nunca única)

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" (funções da TUI) e "Arquitetura do Sistema" (componente `internal/tui/sidebar.go`). Atividade corrente é transiente e chega por parâmetro/campo do Model — o mapa `personaActivity` é mantido na 5.0; aqui o render aceita a atividade que receber e trata vazio como "—". Status em inglês: `waiting`/`deliberating`/`done`.

## Critérios de Sucesso

- Clamp: 100 cols → 24, 140 → 35, 200 → 40; abaixo de 100 cols → 0
- Render com Mesa populada contém nome e status textual de cada persona, modelo, tokens, custo e rodapé com tetos
- Conteúdo de 200 caracteres não estoura a largura (elisão com "…", `lipgloss.Width` == width)
- Mesa nil renderiza estado de mesa não iniciada
- `go test ./internal/tui/` verde

## Testes da Tarefa

- [ ] Testes de unidade (`sidebar_test.go`):
  - Matriz de largura: 80→0, 100→24, 140→35, 200→40, 250→40
  - Conteúdo com Mesa injetada via `RestoreMesa`: nome + status textual presentes; rodapé `n/max` e `k/budget` corretos
  - Elisão: nome/atividade longos truncados com "…", `lipgloss.Width` da linha == largura do painel
  - Mesa nil → "mesa não iniciada" textual
  - Persona sem atividade → "—"
  - Cor nunca única: status textual presente independentemente do estilo aplicado
- [ ] Testes de integração: N/A (render isolado; composição na 5.0)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/sidebar.go` (novo)
- `internal/tui/sidebar_test.go` (novo)
- `internal/tui/theme.go` (modificado — estilos do sidebar)
- `internal/tui/tui.go` (referência — Model, `truncate`, `disciplineColor`)
