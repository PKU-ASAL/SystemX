
# Prompt Input

The `PromptInput` component allows a user to send a message with file attachments to a large language model. It includes a textarea, file upload capabilities, a submit button, and a dropdown for selecting the model.

<Preview path="prompt-input" />

## Installation

```sh
npx ai-elements@latest add prompt-input
```

## Usage

```tsx
import {
  PromptInput,
  PromptInputActionAddAttachments,
  PromptInputActionMenu,
  PromptInputActionMenuContent,
  PromptInputActionMenuItem,
  PromptInputActionMenuTrigger,
  PromptInputAttachment,
  PromptInputAttachments,
  PromptInputBody,
  PromptInputButton,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputToolbar,
  PromptInputTools,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input';
```

```tsx
<PromptInput onSubmit={() => {}} className="mt-4 relative">
  <PromptInputBody>
    <PromptInputAttachments>
      {(attachment) => (
        <PromptInputAttachment data={attachment} />
      )}
    </PromptInputAttachments>
    <PromptInputTextarea onChange={(e) => {}} value={''} />
  </PromptInputBody>
  <PromptInputToolbar>
    <PromptInputTools>
      <PromptInputActionMenu>
        <PromptInputActionMenuTrigger />
        <PromptInputActionMenuContent>
          <PromptInputActionAddAttachments />
        </PromptInputActionMenuContent>
      </PromptInputActionMenu>
    </PromptInputTools>
    <PromptInputSubmit
      disabled={false}
      status={'ready'}
    />
  </PromptInputToolbar>
</PromptInput>
```

## Usage with AI SDK

Built a fully functional chat app using `PromptInput`, [`Conversation`](/elements/components/conversation) with a model picker:

Add the following component to your frontend:

```tsx filename="app/page.tsx"
'use client';

import {
  PromptInput,
  PromptInputActionAddAttachments,
  PromptInputActionMenu,
  PromptInputActionMenuContent,
  PromptInputActionMenuTrigger,
  PromptInputAttachment,
  PromptInputAttachments,
  PromptInputBody,
  PromptInputButton,
  type PromptInputMessage,
  PromptInputModelSelect,
  PromptInputModelSelectContent,
  PromptInputModelSelectItem,
  PromptInputModelSelectTrigger,
  PromptInputModelSelectValue,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputToolbar,
  PromptInputTools,
} from '@/components/ai-elements/prompt-input';
import { GlobeIcon, MicIcon } from 'lucide-react';
import { useState } from 'react';
import { useChat } from '@ai-sdk/react';
import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation';
import { Message, MessageContent } from '@/components/ai-elements/message';
import { Response } from '@/components/ai-elements/response';

const models = [
  { id: 'gpt-4o', name: 'GPT-4o' },
  { id: 'claude-opus-4-20250514', name: 'Claude 4 Opus' },
];

const InputDemo = () => {
  const [text, setText] = useState<string>('');
  const [model, setModel] = useState<string>(models[0].id);
  const [useMicrophone, setUseMicrophone] = useState<boolean>(false);
  const [useWebSearch, setUseWebSearch] = useState<boolean>(false);

  const { messages, status, sendMessage } = useChat();

  const handleSubmit = (message: PromptInputMessage) => {
    const hasText = Boolean(message.text);
    const hasAttachments = Boolean(message.files?.length);

    if (!(hasText || hasAttachments)) {
      return;
    }

    sendMessage(
      { 
        text: message.text || 'Sent with attachments',
        files: message.files 
      },
      {
        body: {
          model: model,
          webSearch: useWebSearch,
        },
      },
    );
    setText('');
  };

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
                      case 'text':
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

        <PromptInput onSubmit={handleSubmit} className="mt-4" globalDrop multiple>
          <PromptInputBody>
            <PromptInputAttachments>
              {(attachment) => <PromptInputAttachment data={attachment} />}
            </PromptInputAttachments>
            <PromptInputTextarea
              onChange={(e) => setText(e.target.value)}
              value={text}
            />
          </PromptInputBody>
          <PromptInputToolbar>
            <PromptInputTools>
              <PromptInputActionMenu>
                <PromptInputActionMenuTrigger />
                <PromptInputActionMenuContent>
                  <PromptInputActionAddAttachments />
                </PromptInputActionMenuContent>
              </PromptInputActionMenu>
              <PromptInputButton
                onClick={() => setUseMicrophone(!useMicrophone)}
                variant={useMicrophone ? 'default' : 'ghost'}
              >
                <MicIcon size={16} />
                <span className="sr-only">Microphone</span>
              </PromptInputButton>
              <PromptInputButton
                onClick={() => setUseWebSearch(!useWebSearch)}
                variant={useWebSearch ? 'default' : 'ghost'}
              >
                <GlobeIcon size={16} />
                <span>Search</span>
              </PromptInputButton>
              <PromptInputModelSelect
                onValueChange={(value) => {
                  setModel(value);
                }}
                value={model}
              >
                <PromptInputModelSelectTrigger>
                  <PromptInputModelSelectValue />
                </PromptInputModelSelectTrigger>
                <PromptInputModelSelectContent>
                  {models.map((model) => (
                    <PromptInputModelSelectItem key={model.id} value={model.id}>
                      {model.name}
                    </PromptInputModelSelectItem>
                  ))}
                </PromptInputModelSelectContent>
              </PromptInputModelSelect>
            </PromptInputTools>
            <PromptInputSubmit disabled={!text && !status} status={status} />
          </PromptInputToolbar>
        </PromptInput>
      </div>
    </div>
  );
};

export default InputDemo;
```

Add the following route to your backend:

```ts filename="app/api/chat/route.ts"
import { streamText, UIMessage, convertToModelMessages } from 'ai';

// Allow streaming responses up to 30 seconds
export const maxDuration = 30;

export async function POST(req: Request) {
  const { 
    model, 
    messages, 
    webSearch 
  }: { 
    messages: UIMessage[]; 
    model: string;
    webSearch?: boolean;
  } = await req.json();

  const result = streamText({
    model: webSearch ? 'perplexity/sonar' : model,
    messages: convertToModelMessages(messages),
  });

  return result.toUIMessageStreamResponse();
}
```

## Features

- Auto-resizing textarea that adjusts height based on content
- File attachment support with drag-and-drop
- Image preview for image attachments
- Configurable file constraints (max files, max size, accepted types)
- Automatic submit button icons based on status
- Support for keyboard shortcuts (Enter to submit, Shift+Enter for new line)
- Customizable min/max height for the textarea
- Flexible toolbar with support for custom actions and tools
- Built-in model selection dropdown
- Responsive design with mobile-friendly controls
- Clean, modern styling with customizable themes
- Form-based submission handling
- Hidden file input sync for native form posts
- Global document drop support (opt-in)

## Props

### `<PromptInput />`

<PropertiesTable
  content={[
    {
      name: 'onSubmit',
      type: '(message: PromptInputMessage, event: FormEvent) => void',
      description: 'Handler called when the form is submitted with message text and files.',
      isOptional: false,
    },
    {
      name: 'accept',
      type: 'string',
      description: 'File types to accept (e.g., "image/*"). Leave undefined for any.',
      isOptional: true,
    },
    {
      name: 'multiple',
      type: 'boolean',
      description: 'Whether to allow multiple file selection.',
      isOptional: true,
    },
    {
      name: 'globalDrop',
      type: 'boolean',
      description: 'When true, accepts file drops anywhere on the document.',
      isOptional: true,
    },
    {
      name: 'syncHiddenInput',
      type: 'boolean',
      description: 'Render a hidden input with given name for native form posts.',
      isOptional: true,
    },
    {
      name: 'maxFiles',
      type: 'number',
      description: 'Maximum number of files allowed.',
      isOptional: true,
    },
    {
      name: 'maxFileSize',
      type: 'number',
      description: 'Maximum file size in bytes.',
      isOptional: true,
    },
    {
      name: 'onError',
      type: '(err: { code: "max_files" | "max_file_size" | "accept", message: string }) => void',
      description: 'Handler for file validation errors.',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLFormElement>',
      description: 'Any other props are spread to the root form element.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputTextarea />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Textarea>',
      description:
        'Any other props are spread to the underlying Textarea component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputToolbar />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description: 'Any other props are spread to the toolbar div.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputTools />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description: 'Any other props are spread to the tools div.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputButton />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Button>',
      description:
        'Any other props are spread to the underlying shadcn/ui Button component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputSubmit />`

<PropertiesTable
  content={[
    {
      name: 'status',
      type: 'ChatStatus',
      description: 'Current chat status to determine button icon (submitted, streaming, error).',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Button>',
      description:
        'Any other props are spread to the underlying shadcn/ui Button component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputModelSelect />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Select>',
      description:
        'Any other props are spread to the underlying Select component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputModelSelectTrigger />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof SelectTrigger>',
      description:
        'Any other props are spread to the underlying SelectTrigger component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputModelSelectContent />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof SelectContent>',
      description:
        'Any other props are spread to the underlying SelectContent component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputModelSelectItem />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof SelectItem>',
      description:
        'Any other props are spread to the underlying SelectItem component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputModelSelectValue />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof SelectValue>',
      description:
        'Any other props are spread to the underlying SelectValue component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputBody />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description: 'Any other props are spread to the body div.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputAttachments />`

<PropertiesTable
  content={[
    {
      name: 'children',
      type: '(attachment: FileUIPart & { id: string }) => React.ReactNode',
      description: 'Render function for each attachment.',
      isOptional: false,
    },
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description: 'Any other props are spread to the attachments container.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputAttachment />`

<PropertiesTable
  content={[
    {
      name: 'data',
      type: 'FileUIPart & { id: string }',
      description: 'The attachment data to display.',
      isOptional: false,
    },
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description: 'Any other props are spread to the attachment div.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputActionMenu />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof DropdownMenu>',
      description:
        'Any other props are spread to the underlying DropdownMenu component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputActionMenuTrigger />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Button>',
      description:
        'Any other props are spread to the underlying Button component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputActionMenuContent />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof DropdownMenuContent>',
      description:
        'Any other props are spread to the underlying DropdownMenuContent component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputActionMenuItem />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof DropdownMenuItem>',
      description:
        'Any other props are spread to the underlying DropdownMenuItem component.',
      isOptional: true,
    },
  ]}
/>

### `<PromptInputActionAddAttachments />`

<PropertiesTable
  content={[
    {
      name: 'label',
      type: 'string',
      description: 'Label for the menu item. Defaults to "Add photos or files".',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof DropdownMenuItem>',
      description:
        'Any other props are spread to the underlying DropdownMenuItem component.',
      isOptional: true,
    },
  ]}
/>

## Hooks

### `usePromptInputAttachments`

Access and manage file attachments within a PromptInput context.

```tsx
const attachments = usePromptInputAttachments();

// Available methods:
attachments.files // Array of current attachments
attachments.add(files) // Add new files
attachments.remove(id) // Remove an attachment by ID
attachments.clear() // Clear all attachments
attachments.openFileDialog() // Open file selection dialog
```
