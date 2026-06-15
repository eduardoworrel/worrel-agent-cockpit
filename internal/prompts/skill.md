# Prompt — Extração de SKILLS (procedimentos reutilizáveis)

Só vira SKILL um PROCEDIMENTO acionável: uma forma de EXECUTAR uma tarefa, com
passos que alguém poderia repetir. Se não há ação a executar, NÃO é skill (vira
memória, ou nada).

Na dúvida entre skill e memória: descreve COMO fazer → skill; descreve ALGO A
LEMBRAR (regra / escolha / fato / armadilha) → memória.

FREQUÊNCIA (priorize o recorrente):
- Detecte quando a MESMA tarefa/intenção se repete em VÁRIAS sessões.
- Para um padrão recorrente emita UMA ÚNICA sugestão (nunca uma por ocorrência).
- Inclua a CONTAGEM de ocorrências na evidence ("repetida em N sessões: ...").
- Priorize alta frequência; ignore eventos isolados sem reuso claro.

WORKFLOWS E TAREFAS PARAMETRIZADAS (não fragmente):
- Um procedimento recorrente de MÚLTIPLAS ETAPAS é UMA ÚNICA skill de workflow —
  não uma skill por etapa.
- Uma tarefa parametrizada recorrente (mesma operação com entrada que varia) é
  UMA ÚNICA skill parametrizada: descreva o parâmetro de entrada e a saída.
- No content (markdown) de um workflow, estruture: etapas NUMERADAS e ordenadas;
  por etapa, o que é ENTRADA e o que é DERIVADO/já disponível; CREDENCIAIS/recursos
  externos exigidos por etapa; e VARIANTES/ramificações conhecidas. Não invente
  etapas que não aparecem nos transcripts.
