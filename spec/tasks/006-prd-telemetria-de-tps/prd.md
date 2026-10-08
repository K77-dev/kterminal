# PRD — Telemetria real de TPS alimentando o catálogo

## Visão Geral

No kterminal, o Jev (router) escolhe o LLM a cada chamada, pesando qualidade, custo e tokens/s. Hoje o TPS que o Jev vê vem de `tps_estimate` — estimativas estáticas no YAML do catálogo. O pitch do produto é "melhor TPS"; medir o TPS real de uso e alimentar a decisão do Jev fecha esse ciclo com dados próprios, coletados no ambiente real do usuário.

Este PRD define a coleta de TPS real por modelo durante o uso, a persistência de estatísticas rolling, e a integração com o catálogo para que o `Criteria()` use TPS medido quando houver amostras suficientes.

## Objetivos

- **Performance**: o Jev decide com TPS medido real, não estimativa estática.
- **Continuidade**: estatísticas sobrevivem a reinícios do app (persistência local).
- **Qualidade dos dados**: amostras absurdas (0, negativas, outliers grosseiros) não contaminam a média.
- **Transparência**: `--doctor` exibe medido vs. estimado lado a lado.

## Histórias de Usuário

- Como Jev (router), quero receber TPS medido real nos critérios dos modelos, para escolher o mais rápido com dados, não com chute.
- Como Jev (usuário), quero que as estatísticas de TPS sobrevivam ao reinício do app, para acumular histórico confiável.
- Como Jev (usuário), quero ver a tabela de TPS medido por modelo no `--doctor`, para comparar com as estimativas do catálogo.

## Funcionalidades Principais

### REQ-001 — Coleta de TPS por modelo

Após cada chamada ao LLM com tokens de output > 0, o TPS real da chamada é registrado por modelo:

- Estatísticas rolling por modelo (média incremental estilo Welford, sem guardar histórico de amostras).
- Janela rolling adicional via EWMA (α=0.3) por modelo.
- Outliers grosseiros (tps ≤ 0 ou > 10000) são descartados.

#### Critérios de Aceite

- Amostras absurdas (0, negativas, >10000) não contaminam a média.
- O valor exposto prioriza o EWMA quando houver ≥ 5 amostras.

---

### REQ-002 — Persistência local

Estatísticas persistidas em `~/.local/share/kterminal/telemetry.json`:

- Carregadas no início da sessão.
- Salvas com debounce (no máximo 1× a cada 10s) e no encerramento.

#### Critérios de Aceite

- Reiniciar o app não perde as estatísticas.

---

### REQ-003 — Integração com o catálogo e o router

- O catálogo expõe, por modelo, o TPS medido calculado (não vem do YAML).
- O agente injeta o valor medido em cada modelo ao montar candidatos para o router.
- O texto de critérios por modelo usa TPS medido (com contagem de chamadas) quando houver amostras suficientes; senão, mantém a estimativa atual do YAML.

#### Critérios de Aceite

- Após 5+ chamadas num modelo, os critérios enviados ao Jev citam o TPS medido.
- Sem amostras suficientes, o texto usa o estimate atual (nunca piora a informação do Jev).

---

### REQ-004 — Visibilidade no `--doctor`

`kterminal --doctor` imprime a tabela de TPS medido por modelo (média, amostras) ao lado das estimativas.

#### Critérios de Aceite

- A tabela do `--doctor` mostra medido e estimado por modelo.

## Experiência do Usuário

- Nada muda visualmente na TUI — o TPS exibido no hint bar já é o medido por resposta.
- O ganho é invisível mas contínuo: as escolhas do Jev melhoram conforme o uso acumula dados reais.
- `--doctor` é a janela de transparência para conferir o que foi medido.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- Sem dependências externas — estatísticas implementadas à mão.
- O YAML continua sendo o fallback — nunca piorar a informação do Jev. Requisito não negociável.
- Persistência segue o padrão existente em `~/.local/share/kterminal/`.

## Fora de Escopo

- Telemetria remota ou compartilhada entre máquinas.
- Gráficos ou visualização de histórico na TUI.
- Medição separada de latência de rede vs. geração.
- Coleta de métricas além de TPS (custo, latência total).
- Configuração de α do EWMA pelo usuário.
