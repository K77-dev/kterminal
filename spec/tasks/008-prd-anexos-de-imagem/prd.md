# PRD — Anexos de imagem no prompt

## Visão Geral

Vários modelos do gateway do kterminal aceitam imagens (o `glm-5.3` tem `attachment: true` na config do opencode), mas o kterminal só troca texto. O usuário não consegue mostrar um screenshot, diagrama ou mock ao agente — um canal inteiro de comunicação está fechado.

Este PRD define o anexo de imagens ao prompt (`/image <caminho>`) e o roteamento automático da chamada apenas para modelos com visão, mantendo o Jev como decidor entre os candidatos válidos.

## Objetivos

- **Qualidade do agente**: o modelo com visão recebe a imagem junto ao prompt e responde sobre ela.
- **Compatibilidade total**: sem anexos, o request ao gateway permanece idêntico ao atual.
- **Roteamento correto**: o Jev só recebe modelos com visão como candidatos quando há imagens.
- **Privacidade**: o base64 da imagem vai só ao gateway LLM — nunca ao Jev nem ao transcript.

## Histórias de Usuário

- Como Jev, quero anexar um screenshot com `/image <caminho>` e perguntar algo sobre ele, para que o agente responda com base no que vê.
- Como Jev, quero ver os anexos pendentes como chips acima do prompt antes de enviar, para confirmar o que será anexado.
- Como Jev, quero remover um anexo por erro sem recomeçar o prompt, para corrigir rapidamente.
- Como Jev, quero um erro claro quando nenhum modelo do gateway tem visão, com meus anexos preservados, para trocar de abordagem sem perder o trabalho.

## Funcionalidades Principais

### REQ-001 — Mensagens com content parts

O formato de mensagem do LLM ganha suporte a partes de conteúdo (texto e imagem):

- Com anexos, a mensagem do usuário vira um array de partes: texto do prompt + imagens codificadas em base64 (formato OpenAI-compatible).
- Sem anexos, o request permanece byte a byte igual ao atual (content como string).

#### Critérios de Aceite

- Mensagem com imagem chega ao gateway como array de content parts.
- Sem anexos, o request é idêntico ao atual (compatibilidade total).

---

### REQ-002 — Catálogo com capacidade de visão

O catálogo de modelos ganha metadado `vision: true|false` por modelo (`glm-5.3`: true; demais: false até confirmação).

#### Critérios de Aceite

- O catálogo expõe a capacidade de visão por modelo, vinda do YAML.

---

### REQ-003 — Comandos de anexo

- `/image <caminho>`: anexa a imagem à próxima mensagem. Valida existência e extensão (png, jpg, jpeg, gif, webp); lê e codifica base64 com limite de 5MB.
- `/image` sem argumentos: lista anexos pendentes.
- `/unimage <n>`: remove o anexo indicado.
- Anexos pendentes aparecem como chips acima do prompt (nome do arquivo, cor secundária).

#### Critérios de Aceite

- Arquivo inexistente ou extensão inválida → erro claro, nada anexado.
- Arquivo acima de 5MB → rejeitado com mensagem clara.
- Chips visíveis acima do prompt; `/unimage` remove o anexo correto.

---

### REQ-004 — Roteamento por visão

Quando a mensagem tem imagens:

- Os routers filtram os candidatos para modelos com `Vision == true` antes de montar a escolha do Jev.
- O estado enviado ao Jev menciona que o passo inclui anexos de imagem.
- Nenhum modelo com visão disponível → erro amigável ("no vision-capable model available"), anexos preservados para o usuário remover.

#### Critérios de Aceite

- Com anexo, o Jev só recebe modelos com `vision: true` como critérios.
- Nenhum modelo de visão no gateway → erro amigável, anexos preservados.

---

### REQ-005 — Transcript sem base64

O transcript grava os anexos como metadados (nome, tamanho) — nunca o base64.

#### Critérios de Aceite

- O JSONL contém metadados dos anexos, sem conteúdo base64.

## Experiência do Usuário

- O fluxo é: `/image screenshot.png` → chip aparece → digitar a pergunta → enter. Nenhum passo extra.
- O bloco do usuário no chat mostra o texto + chips dos arquivos anexados — o histórico permanece legível.
- Erros de anexo (arquivo grande, formato inválido) aparecem inline, sem abortar o prompt.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- Formato de request OpenAI-compatible (array de content parts com `image_url`).
- **Não enviar base64 ao Jev nem ao transcript** — só ao gateway LLM. Requisito não negociável de privacidade.
- Limite de 5MB por imagem.

## Fora de Escopo

- Colar imagem direto do clipboard.
- Captura de screenshot integrada.
- Anexos de outros tipos (PDF, vídeo, áudio).
- OCR local antes do envio.
- Redimensionamento/compressão automática de imagens grandes.
- Visão em mensagens do agente (só mensagens do usuário).
