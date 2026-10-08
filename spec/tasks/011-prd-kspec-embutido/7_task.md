# Tarefa 7.0: Diagnóstico no --doctor e verificação final do fluxo SDD

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-009 — Diagnóstico (`--doctor`, `/kspec-version`)

## Dependências

- 3.0, 5.0, 6.0

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechar o REQ-009 com a seção kspec no `--doctor` (versão embutida + fonte ativa no projeto corrente) e a verificação final de integração do fluxo SDD completo: ativação de skill → system prompt na chamada → pergunta via `ask_user` → artefato gravado em `spec/tasks/` do projeto corrente.

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md`, `tests.md`.
</skills>

<requirements>
- `--doctor` exibe `kspec: v{X.Y.Z} ({source})` — "embedded" ou "project" conforme a fonte ativa no cwd
- Fluxo SDD completo verificado em teste de integração com fake LLM
- Verificação completa do repositório passa
</requirements>

## Subtarefas

- [ ] 7.1 Adicionar a seção kspec ao `runDoctor` em `main.go`, usando `Version()`/`Source()` do Store
- [ ] 7.2 Escrever teste de integração do fluxo completo (fake LLM): `/kspec-*` ativa skill → system prompt contém skill + rules → `ask_user` responde via canal → artefato gravado em `spec/tasks/` do projeto corrente (temp dir)
- [ ] 7.3 Executar a verificação completa: `go build ./... && go vet ./... && gofmt -l . && go test ./...`

## Detalhes de Implementação

Seguir a techspec, seção "Monitoramento e Observabilidade → Health Checks". O doctor não faz chamadas de rede para o kspec — apenas reporta versão e fonte. O checklist E2E manual (fluxo em projeto vazio, precedência, bootstrap validado no Claude Code, `--continue` com skill ativa) fica documentado como critério de aceite do QA, não como teste automatizado.

## Critérios de Sucesso

- `--doctor` exibe "kspec vX.Y.Z (embedded)" ou "(project)" conforme a fonte ativa
- Teste de integração do fluxo SDD passa end-to-end com fake LLM
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passa sem falhas

## Testes da Tarefa

- [ ] Testes de unidade
  - Formatação da linha kspec do doctor nos dois modos de fonte (se extraída em função testável; senão verificar via teste de integração)
- [ ] Testes de integração
  - Fluxo completo: ativação → system prompt → ask_user → artefato em `spec/tasks/` (fake LLM, temp dir)
- [ ] Testes E2E (se aplicável)
  - Checklist manual para o QA: `/kspec-prd` em projeto vazio (fluxo completo com `ask_user`, artefato em `spec/tasks/`); projeto com `.agents/` (precedência); bootstrap visível no Claude Code; `--continue` com skill ativa; `--doctor` (exibe `kspec: v{X.Y.Z} (embedded|project)` e WARNINGs de skills quebradas); duplo enter no popup de comandos (enter com popup aberto completa, segundo enter executa — precedente do popup de menções); popup de comandos com os ~20 itens em terminal curto (< ~27 rows) — o renderer corta o topo da view nessa condição (comportamento herdado do popup de menções)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go` (`runDoctor`)
- `main_test.go`
- `internal/agent/agent_test.go` (teste de integração do fluxo)
- `spec/tasks/011-prd-kspec-embutido/prd.md` (critérios de aceite para o QA)
