# PRD — Diagnósticos pós-edição (go vet hook)

## Visão Geral

No kterminal, o agente edita arquivos Go com `write`/`edit` e só descobre que quebrou o código quando o usuário reclama — o feedback loop de qualidade é mediado por humano, o mais lento possível. Um LSP completo é infraestrutura pesada para um primeiro passo; um hook de diagnóstico pós-edição com `go vet` dá retorno imediato ao agente **no mesmo turno**.

Este PRD define: depois de toda edição em arquivo `.go`, rodar `go vet` no pacote afetado e anexar o diagnóstico ao resultado da tool — o próprio modelo corrige o erro no passo seguinte, sem intervenção humana.

## Objetivos

- **Qualidade do agente**: o agente descobre e corrige os próprios erros no mesmo turno, sem intervenção humana.
- **Autonomia**: feedback explícito também quando está tudo certo ("clean"), para o modelo saber que pode seguir.
- **Robustez**: o diagnóstico é best-effort — nenhum problema de ambiente (Go ausente, rede, timeout) pode falhar a edição em si.
- **Foco**: diagnóstico apenas do arquivo editado, sem ruído do restante do repositório.

## Histórias de Usuário

- Como Jev, quero que o agente receba o diagnóstico do `go vet` logo após editar um arquivo Go, para que ele corrija o erro sozinho no passo seguinte.
- Como Jev, quero confirmação explícita de "clean" quando a edição não introduz problemas, para confiar que o agente sabe que está tudo bem.
- Como Jev, quero que edições em arquivos não-Go ou fora de módulo Go não rodem diagnóstico, para não pagar custo sem benefício.
- Como Jev, quero que a ausência de Go no ambiente não quebre as edições, para que a tool funcione em qualquer máquina.

## Funcionalidades Principais

### REQ-001 — Hook pós-edição em arquivos Go

Após `write`/`edit` bem-sucedidos em arquivos `.go` localizados dentro de um módulo Go (detectado pela presença de `go.mod` subindo diretórios), um hook de diagnóstico executa automaticamente.

#### Critérios de Aceite

- Edit em arquivo `.go` dentro de módulo dispara o diagnóstico.
- Edit em arquivo não-Go não roda vet.
- Edit fora de módulo Go não roda vet.

---

### REQ-002 — Diagnóstico best-effort com `go vet`

- `go vet` roda no pacote do arquivo editado, com timeout de 30s.
- A saída é filtrada para o arquivo editado (linhas com o caminho relativo do arquivo).
- Falhas de ambiente (Go ausente no PATH, rede, timeout) retornam silenciosamente sem diagnóstico — a edição em si nunca falha por causa do hook.
- Timeout estourado: o processo é morto e só o output original da edição é retornado.

#### Critérios de Aceite

- Ambiente sem `go` no PATH → tool funciona normalmente sem diagnóstico.
- `go vet` que demora mais que 30s não bloqueia indefinidamente.

---

### REQ-003 — Feedback explícito ao modelo

- Com diagnóstico: as linhas de erro relevantes são anexadas ao resultado da tool em bloco `Diagnostics:`.
- Sem diagnóstico: anexar `Diagnostics: clean` — feedback explícito para o modelo saber que pode prosseguir.

#### Critérios de Aceite

- Edit que introduz `undefined: Foo` → resultado da tool contém a linha do erro; o modelo corrige no passo seguinte.
- Edit válido → resultado contém `Diagnostics: clean`.

---

### REQ-004 — Correção autônoma no mesmo turno

O diagnóstico anexado ao resultado da tool circula de volta ao LLM como parte da mensagem de resultado — o modelo reage ao texto e corrige o erro no passo seguinte, fechando o loop sem intervenção humana.

#### Critérios de Aceite

- Em um turno completo, uma edição que quebra o código é seguida de correção pelo próprio agente, sem input do usuário.

## Experiência do Usuário

- Nenhuma mudança visual na TUI: o diagnóstico aparece no resultado da tool como qualquer outro output, com o truncamento normal.
- A experiência percebida é um agente que "se auto-corrige": edita, vê o erro, corrige — tudo visível no chat.
- O usuário para de ser o detector de erros do próprio agente.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- Best-effort: **nenhum erro de diagnóstico pode falhar a tool em si**. Requisito não negociável.
- Sem dependências externas; sem LSP — `go vet` puro.
- O hook é injetado no registry na inicialização (main), mantendo o pacote de tools testável sem Go instalado.
- Timeout de 30s por execução de diagnóstico.

## Fora de Escopo

- LSP completo (diagnósticos incrementais, multi-linguagem).
- Diagnóstico para outras linguagens (TypeScript, Python etc.).
- Formatação automática (gofmt) ou fixes automáticos aplicados sem o modelo.
- Diagnóstico de arquivos não editados no turno.
- `go build`/`go test` como parte do hook (só `go vet`).
