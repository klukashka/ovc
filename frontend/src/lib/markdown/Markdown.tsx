import { type ReactElement, type ReactNode, useMemo } from 'react';

import { parseMarkdown } from './parse.ts';
import type { BlockNode, InlineNode } from './syntax.ts';

function renderInline(nodes: readonly InlineNode[]): ReactNode[] {
  return nodes.map((node, index) => {
    switch (node.kind) {
      case 'text':
        return node.text;
      case 'break':
        return <br key={index} />;
      case 'code':
        return (
          <code key={index} className="rounded-sm bg-hover px-1 font-mono text-small">
            {node.text}
          </code>
        );
      case 'emphasis':
        return <em key={index}>{renderInline(node.children)}</em>;
      case 'strong':
        return (
          <strong key={index} className="font-semibold">
            {renderInline(node.children)}
          </strong>
        );
      case 'link':
        return (
          <a
            key={index}
            href={node.href}
            target="_blank"
            rel="noopener noreferrer"
            className="text-accent-text underline [overflow-wrap:anywhere]"
          >
            {renderInline(node.children)}
          </a>
        );
    }
  });
}

function renderBlock(block: BlockNode, index: number): ReactElement {
  switch (block.kind) {
    case 'paragraph':
      return (
        <p key={index} dir="auto" className="[overflow-wrap:anywhere]">
          {renderInline(block.content)}
        </p>
      );
    case 'code_block':
      return (
        <pre key={index} dir="ltr" className="overflow-x-auto rounded-sm bg-hover p-3 font-mono text-small">
          <code>{block.text}</code>
        </pre>
      );
    case 'unordered_list':
      return (
        <ul key={index} className="list-disc ps-6">
          {block.items.map((item, itemIndex) => (
            <li key={itemIndex} dir="auto" className="[overflow-wrap:anywhere]">
              {renderInline(item)}
            </li>
          ))}
        </ul>
      );
    case 'ordered_list':
      return (
        <ol key={index} start={block.start} className="list-decimal ps-6">
          {block.items.map((item, itemIndex) => (
            <li key={itemIndex} dir="auto" className="[overflow-wrap:anywhere]">
              {renderInline(item)}
            </li>
          ))}
        </ol>
      );
  }
}

export function Markdown({ source }: { readonly source: string }): ReactElement {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  return <div className="flex flex-col gap-3">{blocks.map(renderBlock)}</div>;
}
