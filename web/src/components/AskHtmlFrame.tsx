import { useEffect, useRef, useState } from 'react';

interface Props {
  html: string;
  // Chamado quando o usuário clica numa opção [data-choice] dentro do HTML rico.
  onChoice: (value: string) => void;
}

// BRIDGE é o script que NÓS injetamos no iframe (o LLM nunca escreve <script>).
// Ele faz duas coisas e nada mais: (1) ao clicar num elemento com data-choice,
// posta o valor pro pai; (2) reporta a altura do conteúdo pro pai dimensionar o
// iframe. Sandbox = allow-scripts SEM allow-same-origin → origem opaca, não
// acessa o app pai; a comunicação é só por postMessage.
const BRIDGE = `<script>(function(){
  function send(){parent.postMessage({source:'worrel-ask-height',height:document.documentElement.scrollHeight},'*');}
  document.addEventListener('click',function(e){
    var el=e.target.closest&&e.target.closest('[data-choice]');
    if(!el)return; e.preventDefault();
    parent.postMessage({source:'worrel-ask-choice',value:el.getAttribute('data-choice')},'*');
  });
  window.addEventListener('load',send); new ResizeObserver(send).observe(document.documentElement); setTimeout(send,50);
})();</script>`;

// AskHtmlFrame renderiza o HTML rico (ask_html) num iframe ISOLADO e torna suas
// opções clicáveis sem expor o app a <script> do modelo. As choices vivem no HTML
// (experiência rica e condensada); o clique volta via postMessage.
export default function AskHtmlFrame({ html, onChoice }: Props) {
  const ref = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(120);

  useEffect(() => {
    function onMessage(e: MessageEvent) {
      if (!ref.current || e.source !== ref.current.contentWindow) return;
      const d = e.data as { source?: string; value?: string; height?: number };
      if (d?.source === 'worrel-ask-choice' && typeof d.value === 'string') onChoice(d.value);
      else if (d?.source === 'worrel-ask-height' && typeof d.height === 'number') {
        setHeight(Math.min(Math.max(d.height + 2, 80), 520));
      }
    }
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [onChoice]);

  return (
    <iframe ref={ref} className="ixp-ask-html" sandbox="allow-scripts"
      style={{ height }} srcDoc={html + BRIDGE} title="ask" />
  );
}
