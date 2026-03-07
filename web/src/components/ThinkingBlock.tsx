import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ChevronDown, ChevronUp } from '@untitledui/icons';

interface ThinkingBlockProps {
  content: string;
}

export function ThinkingBlock({ content }: ThinkingBlockProps) {
  const { t } = useTranslation();
  const [isExpanded, setIsExpanded] = useState(false);

  if (!content.trim()) return null;

  return (
    <div className="mb-3">
      {/* Toggle button */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="text-text-muted hover:text-text-secondary flex cursor-pointer items-center gap-2 text-sm transition-colors"
      >
        <span>{t('chat.showReasoning')}</span>
        {isExpanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
      </button>

      {/* Content */}
      {isExpanded && (
        <>
          <div className="text-text-secondary mt-2 text-sm">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                // Estilos simplificados para o markdown dentro do thinking
                p: ({ children }) => <p className="mb-2 leading-5 last:mb-0">{children}</p>,
                strong: ({ children }) => (
                  <strong className="text-text-primary font-medium">{children}</strong>
                ),
                ul: ({ children }) => <ul className="my-2 ml-4 list-disc space-y-1">{children}</ul>,
                ol: ({ children }) => (
                  <ol className="my-2 ml-4 list-decimal space-y-1">{children}</ol>
                ),
                li: ({ children }) => <li className="leading-5">{children}</li>,
                code: ({ className, children, ...props }) => {
                  const isInline = !className;
                  if (isInline) {
                    return (
                      <code className="bg-bg-elevated rounded px-1 py-0.5 text-xs" {...props}>
                        {children}
                      </code>
                    );
                  }
                  return (
                    <code className={className} {...props}>
                      {children}
                    </code>
                  );
                },
              }}
            >
              {content}
            </ReactMarkdown>
          </div>
          {/* Divider */}
          <div className="border-border mt-3 h-px w-full border-b" />
        </>
      )}
    </div>
  );
}

/**
 * Extrai conteúdo de tags de pensamento de uma string.
 * Retorna { thinkingContent, cleanContent }
 * - thinkingContent: conteúdo extraído das tags (agrupado se múltiplas)
 * - cleanContent: conteúdo original sem as tags de pensamento
 */
export function extractThinkingContent(text: string): {
  thinkingContent: string;
  cleanContent: string;
} {
  // Regex para detectar as tags: <think />, <thinking>, <reasoning>, <thought>
  // Case-insensitive
  const thinkingRegex =
    /<think\s*\/>|<think(?:ing)?>([\s\S]*?)<\/think(?:ing)?>|<reasoning>([\s\S]*?)<\/reasoning>|<thought>([\s\S]*?)<\/thought>/gi;

  const thinkingParts: string[] = [];
  let match;

  // Extrair todo o conteúdo das tags fechadas
  while ((match = thinkingRegex.exec(text)) !== null) {
    // match[0] é o match completo
    // match[1], match[2], match[3] são os grupos de captura
    const content = match[1] || match[2] || match[3] || '';
    if (content.trim()) {
      thinkingParts.push(content.trim());
    }
  }

  // Remover as tags fechadas do conteúdo original
  let cleanContent = text.replace(thinkingRegex, '').trim();

  // Detectar tags incompletas (streaming) - tag aberta mas não fechada
  // Isso acontece durante o streaming quando <thinking> foi aberta mas </thinking> ainda não chegou
  const openTagRegex = /<(think(?:ing)?|reasoning|thought)>([^]*?)$/i;
  const openMatch = cleanContent.match(openTagRegex);

  if (openMatch) {
    // openMatch[1] é o nome da tag, openMatch[2] é o conteúdo parcial
    const partialContent = openMatch[2] || '';
    if (partialContent.trim()) {
      thinkingParts.push(partialContent.trim());
    }
    // Remover a tag incompleta e seu conteúdo do cleanContent
    cleanContent = cleanContent.replace(openTagRegex, '').trim();
  }

  return {
    thinkingContent: thinkingParts.join('\n\n'),
    cleanContent,
  };
}
