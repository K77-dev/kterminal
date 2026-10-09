# Tarefa 1.0: Config — knobs de resiliência

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Configuração e diagnóstico (parte de parsing/validação; wiring fica na 5.0)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Expor os knobs de resiliência em `config.toml`: `[llm]` ganha `request_timeout`, `idle_timeout`, `first_byte_timeout` (strings de duração estilo `"10m"`/`"90s"`) e `max_retries` (int); nova seção `[agent]` com `subagent_timeout`. Defaults, env overrides e validação com erro claro acontecem em `Load()`.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, config TOML via BurntSushi, XDG.
</skills>

<requirements>
- Campos TOML com nomes exatos: `request_timeout`, `idle_timeout`, `first_byte_timeout`, `max_retries` em `[llm]`; `subagent_timeout` em `[agent]`
- Env: `KTERMINAL_LLM_REQUEST_TIMEOUT`, `KTERMINAL_LLM_IDLE_TIMEOUT`, `KTERMINAL_LLM_FIRST_BYTE_TIMEOUT`, `KTERMINAL_LLM_MAX_RETRIES`, `KTERMINAL_AGENT_SUBAGENT_TIMEOUT`
- Defaults: request 10m, idle 5m, first byte 60s, retries 2, subagent 10m
- `"0s"` em request_timeout desliga a capa; `max_retries` vazio mantém default, negativo rejeitado
- Duração inválida → erro de boot nomeando a chave
</requirements>

## Subtarefas

- [ ] 1.1 Adicionar campos a `config.LLM` e criar `config.Agent` com `SubagentTimeout string`; montar em `Config`
- [ ] 1.2 Criar helper de parsing de duração (string → `time.Duration` via `time.ParseDuration`) com erro apontando a chave TOML
- [ ] 1.3 Aplicar defaults e env overrides em `Load()`; validar `max_retries >= 0`
- [ ] 1.4 Testes: defaults sem config; parsing `"90s"`/`"10m"`/`"0s"`; duração inválida com chave no erro; env prevalece sobre arquivo; `max_retries` negativo rejeitado

## Detalhes de Implementação

Ver techspec.md — seções "Modelos de Dados" e "Pontos de Integração". Os valores parseados ficam disponíveis para o wiring (task 5.0) — expor via campos públicos `time.Duration`/`int` já resolvidos em `Config` (ex: `cfg.LLM.IdleTimeoutDuration`), mantendo as strings TOML como campo bruto.

## Critérios de Sucesso

- `config.Load()` sem arquivo retorna todos os defaults resolvidos
- Arquivo com `idle_timeout = "5min"` falha o boot com mensagem citando `llm.idle_timeout`
- Env `KTERMINAL_LLM_IDLE_TIMEOUT=90s` prevalece sobre o arquivo
- `go test ./internal/config/` verde

## Testes da Tarefa

- [ ] Testes de unidade: parsing, defaults, env, validação (subtarefas 1.4)
- [ ] Testes de integração: N/A (parsing puro)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/config/config.go`
- `internal/config/config_test.go`
