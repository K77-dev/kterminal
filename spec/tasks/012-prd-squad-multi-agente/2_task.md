# Tarefa 2.0: Seção `[squad]` no config TOML — defaults, pins e `default_mode`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Seletor de modo `/mode`
- REQ-005 — Roteamento por papel e pin manual
- REQ-006 — Kickoff, tetos e critério de saída

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: < 2h

## Visão Geral

Estender `internal/config` com a seção `[squad]` no TOML: `default_mode`, `max_convocations`, `token_budget` e a subseção `[squad.pins]` (papel → modelo). Aplicar defaults sensatos (8 convocações / 200.000 tokens / `sdd`) quando a seção estiver ausente e rejeitar `default_mode` inválido.

## Conformidade com Skills Padrões

- Go 1.27, módulo único `kterminal`
- Config TOML em XDG (`~/.config/kterminal/config.toml`) via `BurntSushi/toml` (já usada)
- Código em inglês, sem comentários
- Precedentes de env override: `KTERMINAL_LLM_*`, `TYPESAFE_API_KEY` — seguir o mesmo padrão se necessário

## Requisitos

- Struct `Squad` com tags toml: `default_mode`, `max_convocations`, `token_budget`, `pins` (map)
- Defaults aplicados quando a seção estiver ausente: `sdd`, 8, 200000
- `default_mode` inválido (não `sdd` nem `squad`) rejeitado com erro claro
- Pins parseados como `map[string]string` (papel → modelo)
- Compatível com config existente (seções `llm`, `typesafe` inalteradas)

## Subtarefas

- [ ] 2.1 Adicionar struct `Squad` em `internal/config/config.go` e integrar ao `Config`
- [ ] 2.2 Aplicar defaults em `Load()` quando a seção estiver ausente
- [ ] 2.3 Validar `default_mode` (apenas `sdd`/`squad`) e retornar erro claro para valor inválido
- [ ] 2.4 Garantir round-trip em `Save()` (marshal da seção `[squad]`)

## Detalhes de Implementação

Consulte "Modelos de Dados" na `techspec.md`:

```toml
[squad]
default_mode = "sdd"
max_convocations = 8
token_budget = 200000

[squad.pins]
architect = "glm-5.3"
```

O `Config` atual (`internal/config/config.go:21`) tem apenas `LLM` e `Typesafe`. Adicionar:

```go
type Squad struct {
    DefaultMode     string            `toml:"default_mode"`
    MaxConvocations int               `toml:"max_convocations"`
    TokenBudget     int64             `toml:"token_budget"`
    Pins            map[string]string `toml:"pins"`
}
```

Constantes de default: `defaultMode = "sdd"`, `defaultMaxConvocations = 8`, `defaultTokenBudget = 200000`.

## Critérios de Sucesso

- Sem seção `[squad]`, `cfg.Squad` retorna defaults (`sdd`, 8, 200000)
- Com seção parcial, campos ausentes caem em defaults
- `default_mode = "invalid"` retorna erro na leitura
- Pins são parseados corretamente
- `Save()` preserva a seção `[squad]`

## Testes da Tarefa

- [ ] Testes de unidade — `internal/config/config_test.go`:
  - Defaults aplicados quando a seção estiver ausente (8 / 200k / `sdd`)
  - Pins parseados de TOML
  - `default_mode` inválido rejeitado
  - Override de tetos via config
- [ ] Testes de integração — round-trip `Save()` → `Load()`

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/config/config.go` (modificado — seção `[squad]`)
- `internal/config/config_test.go` (novo/modificado)
