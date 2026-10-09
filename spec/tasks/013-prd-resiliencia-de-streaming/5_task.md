# Tarefa 5.0: Wiring de config + doctor

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Configuração e diagnóstico (wiring e doctor)
- REQ-005 — Erros acionáveis na TUI (valores visíveis para diagnóstico)

## Dependências

- 1.0, 2.0, 3.0, 4.0

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Ligar os knobs do config ao runtime: `main.go` injeta os limites no `llm.Client` (`SetLimits`) e no `Agent` (`SetSubagentTimeout`); `--doctor` imprime os valores efetivos. Verificação completa do repo fecha a feature.

<skills>
### Conformidade com Skills Padrões

Padrões do código-base Go (AGENTS.md): sem comentários; wiring centralizado em `main()` como os setters existentes (`SetSquadPins`, `SetSquadLimits`).
</skills>

<requirements>
- `main()` aplica `cfg` → `llmClient.SetLimits(...)` e `ag.SetSubagentTimeout(...)` antes da TUI subir
- `--doctor` imprime `llm timeouts: first byte 60s, idle 5m0s, total 10m0s, retries 2` e `agent: subagent stall 10m0s` na seção de config
- Doctor usa os mesmos valores efetivos do runtime (não recalcula defaults)
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` tudo verde
</requirements>

## Subtarefas

- [ ] 5.1 Injetar `SetLimits`/`SetSubagentTimeout` em `main()` (cliente criado apenas quando `cfg.Ready()`)
- [ ] 5.2 Imprimir valores efetivos no `runDoctor`
- [ ] 5.3 Rodar verificação completa do repo e corrigir o que apontar

## Detalhes de Implementação

Ver techspec.md — seção "Pontos de Integração". O doctor cria um `llm.Client` próprio para o check de gateway; aplicar os mesmos limites nele para o check refletir a realidade.

## Critérios de Sucesso

- Config com `idle_timeout = "90s"` → doctor mostra `idle 1m30s`
- Sem config → doctor mostra os defaults
- Verificação completa verde

## Testes da Tarefa

- [ ] Testes de unidade: N/A (wiring); doctor é saída de texto
- [ ] Testes de integração: `main_test.go` existente continua verde
- [ ] Testes E2E: manual — `kterminal --doctor` com e sem config custom

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go`
- `main_test.go`
