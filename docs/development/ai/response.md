
# Response

The `Response` component renders a Markdown response from a large language model. It uses [Streamdown](https://streamdown.ai/) under the hood to render the markdown.

<Preview path="response" />

## Installation

```sh
npx ai-elements@latest add response
```

After adding the component, you'll need to add the following to your `globals.css` file:

```css
@source "../node_modules/streamdown/dist/index.js";
```

This will ensure that the Streamdown styles are applied to your project. See [Streamdown's documentation](https://streamdown.ai/) for more details.

## Usage

```tsx
import { Response } from '@/components/ai-elements/response';
```

```tsx
<Response>**Hi there.** I am an AI model designed to help you.</Response>
```

## Usage with AI SDK

Populate a markdown response with messages from [`useChat`](/docs/reference/ai-sdk-ui/use-chat).

Add the following component to your frontend:

```tsx filename="app/page.tsx"
'use client';

import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation';
import { Message, MessageContent } from '@/components/ai-elements/message';
import { useChat } from '@ai-sdk/react';
import { Response } from '@/components/ai-elements/response';

const ResponseDemo = () => {
  const { messages } = useChat();

  return (
    <div className="max-w-4xl mx-auto p-6 relative size-full rounded-lg border h-[600px]">
      <div className="flex flex-col h-full">
        <Conversation>
          <ConversationContent>
            {messages.map((message) => (
              <Message from={message.role} key={message.id}>
                <MessageContent>
                  {message.parts.map((part, i) => {
                    switch (part.type) {
                      case 'text': // we don't use any reasoning or tool calls in this example
                        return (
                          <Response key={`${message.id}-${i}`}>
                            {part.text}
                          </Response>
                        );
                      default:
                        return null;
                    }
                  })}
                </MessageContent>
              </Message>
            ))}
          </ConversationContent>
          <ConversationScrollButton />
        </Conversation>
      </div>
    </div>
  );
};

export default ResponseDemo;
```

## Features

- Renders markdown content with support for paragraphs, links, and code blocks
- Supports GFM features like tables, task lists, and strikethrough text via remark-gfm
- Supports rendering Math Equations via rehype-katex
- **Smart streaming support** - automatically completes incomplete formatting during real-time text streaming
- Code blocks are rendered with syntax highlighting for various programming languages
- Code blocks include a button to easily copy code to clipboard
- Adapts to different screen sizes while maintaining readability
- Seamlessly integrates with both light and dark themes
- Customizable appearance through className props and Tailwind CSS utilities
- Built with accessibility in mind for all users

## Props

### `<Response />`

<PropertiesTable
  content={[
    {
      name: 'children',
      type: 'string',
      description: 'The markdown content to render.',
    },
    {
      name: 'parseIncompleteMarkdown',
      type: 'boolean',
      description: 'Whether to parse and fix incomplete markdown syntax (e.g., unclosed code blocks or lists).',
      default: 'true',
      isOptional: true,
    },
    {
      name: 'className',
      type: 'string',
      description: 'CSS class names to apply to the wrapper div element.',
      isOptional: true,
    },
    {
      name: 'components',
      type: 'object',
      description: 'Custom React components to use for rendering markdown elements (e.g., custom heading, paragraph, code block components).',
      isOptional: true,
    },
    {
      name: 'allowedImagePrefixes',
      type: 'string[]',
      description: 'Array of allowed URL prefixes for images. Use ["*"] to allow all images.',
      default: '["*"]',
      isOptional: true,
    },
    {
      name: 'allowedLinkPrefixes',
      type: 'string[]',
      description: 'Array of allowed URL prefixes for links. Use ["*"] to allow all links.',
      default: '["*"]',
      isOptional: true,
    },
    {
      name: 'defaultOrigin',
      type: 'string',
      description: 'Default origin to use for relative URLs in links and images.',
      isOptional: true,
    },
    {
      name: 'rehypePlugins',
      type: 'array',
      description: 'Array of rehype plugins to use for processing HTML. Includes KaTeX for math rendering by default.',
      default: '[rehypeKatex]',
      isOptional: true,
    },
    {
      name: 'remarkPlugins',
      type: 'array',
      description: 'Array of remark plugins to use for processing markdown. Includes GitHub Flavored Markdown and math support by default.',
      default: '[remarkGfm, remarkMath]',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description: 'Any other props are spread to the root div.',
      isOptional: true,
    },
  ]}
/>
