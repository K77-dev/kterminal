# Tarefa 8.0: Verificação final — suite completa com `-race` e checagem manual das métricas do PRD

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Lançamento em background via tool `bash` (métricas: retorno <1s, comportamento síncrono intacto)
- REQ-002 — Contador dinâmico no rodapé (métrica: contador == processos vivos em 100% das observações)
- REQ-003 — Seletor interativo para parar monitoramentos (verificação do fluxo completo)
- REQ-004 — Encerramento na saída (métrica: zero órfãos)
- REQ-005 — Status bar enxuta (métrica: ≤4 segmentos persistentes na sessão típica)

## Dependências

- 6.0 (popup `/bg` completo)
- 7.0 (wiring e shutdown completos)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: < 2h

## Visão Geral

Fechamento da entrega: rodar a verificação completa especificada no "Sequenciamento de Desenvolvimento" da techspec (passo 6) e validar manualmente as quatro métricas de sucesso do PRD. O `-race` é essencial — procmon é o primeiro código concorrente de processo no repo. Nenhum código novo é escrito nesta task, salvo correção de falhas que a verificação revelar.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): comandos do projeto (`go build`, `go vet`, `gofmt`, `go test`), verificação E2E manual por transcript JSONL (TestSprite não se aplica — TUI Go sem frontend web).
</skills>

<requirements>
- `go build ./... && go vet ./... && gofmt -l .` limpos (gofmt sem output)
- `go test ./... -race` verde na suite completa
- Split de plataforma compilável: `GOOS=windows go build ./internal/procmon/` (stubs `!unix`)
- Checagem manual das 4 métricas do PRD (roteiro em "Testes da Tarefa")
- Nenhuma dependência nova em go.mod; `golang.org/x/sys` permanece `// indirect`
</requirements>

## Subtarefas

- [ ] 8.1 Rodar `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./... -race` — tudo verde
- [ ] 8.2 Verificar compilação cross-platform do split (`GOOS=windows go build ./internal/procmon/`)
- [ ] 8.3 Executar o roteiro manual das 4 métricas do PRD (contador == vivos, retorno <1s, zero órfãos, barra ≤4 segmentos) e registrar o resultado
- [ ] 8.4 Corrigir qualquer falha revelada e re-rodar a suite

## Detalhes de Implementação

Consulte techspec.md — seções "Sequenciamento de Desenvolvimento" (passo 6, verificação final), "Abordagem de Testes → Testes de E2E" (roteiro manual por transcript/`ps`) e "Infraestrutura" (rollback e restrições de dependências).

## Critérios de Sucesso

- Suite completa verde com `-race`, `gofmt` sem output, `go vet` limpo
- As 4 métricas do PRD validadas manualmente:
  1. Contador do rodapé == contagem real de processos vivos (rodapé contra `ps` com 2 processos ativos)
  2. Chamada `bash` com `run_in_background` retornando em <1s (cronometrar a tool call no transcript)
  3. Nenhum processo sobrevivendo ao fechamento da TUI (`ps` após quit com 2 monitoramentos)
  4. Sessão típica (squad, jev on, sem pin, sem monitoramentos) com no máximo 4 segmentos persistentes na barra (modelo, custo, tok/s e nada além do essencial)
- Critérios de aceite REQ-005 confirmados: `/jev`, `--doctor`, `/config` e `/mode` continuam refletindo a informação removida da barra

## Testes da Tarefa

- [ ] Testes de unidade: N/A (execução da suite existente — tasks 1.0–7.0 já criaram todos os testes)
- [ ] Testes de integração: N/A (idem)
- [ ] Testes E2E (roteiro manual, por transcript JSONL e `ps` — TestSprite não aplicável):
  - [ ] Métrica 1 — contador == vivos: subir 2 processos em background, comparar `bg 2` no rodapé contra `ps`; parar um via `/bg`, conferir `bg 1` contra `ps`
  - [ ] Métrica 2 — retorno <1s: cronometrar a tool call `bash {run_in_background: true}` no transcript (timestamps de `tool_call`/`tool_result`)
  - [ ] Métrica 3 — zero órfãos: `/quit` com 2 monitoramentos ativos, `ps` imediatamente após; repetir com Ctrl+C e com `kill -TERM`
  - [ ] Métrica 4 — barra enxuta: sessão típica (squad, jev on, sem pin, sem bg) com ≤4 segmentos; `jev off` visível como desvio; pin ativo renderiza `⚑ <model>` uma única vez
  - [ ] Descoberta preservada: `/jev`, `--doctor`, `/config` e `/mode` refletem a informação removida da barra

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- Nenhum arquivo novo (verificação; correções pontuais nos arquivos das tasks 1.0–7.0 se a verificação revelar falhas)
- `spec/tasks/016-prd-monitoramento-background/prd.md` (referência — métricas de sucesso e critérios de aceite)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — sequenciamento, passo 6)
