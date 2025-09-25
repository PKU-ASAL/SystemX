
# Chain of Thought

The `ChainOfThought` component provides a visual representation of an AI's reasoning process, showing step-by-step thinking with support for search results, images, and progress indicators. It helps users understand how AI arrives at conclusions.

<Preview path="chain-of-thought" />

## Installation

```sh
npx ai-elements@latest add chain-of-thought
```

## Usage

```tsx
import {
  ChainOfThought,
  ChainOfThoughtContent,
  ChainOfThoughtHeader,
  ChainOfThoughtImage,
  ChainOfThoughtSearchResult,
  ChainOfThoughtSearchResults,
  ChainOfThoughtStep,
} from '@/components/ai-elements/chain-of-thought';
```

```tsx
<ChainOfThought defaultOpen>
  <ChainOfThoughtHeader />
  <ChainOfThoughtContent>
    <ChainOfThoughtStep
      icon={SearchIcon}
      label="Searching for information"
      status="complete"
    >
      <ChainOfThoughtSearchResults>
        <ChainOfThoughtSearchResult>
          Result 1
        </ChainOfThoughtSearchResult>
      </ChainOfThoughtSearchResults>
    </ChainOfThoughtStep>
  </ChainOfThoughtContent>
</ChainOfThought>
```

## Features

- Collapsible interface with smooth animations powered by Radix UI
- Step-by-step visualization of AI reasoning process
- Support for different step statuses (complete, active, pending)
- Built-in search results display with badge styling
- Image support with captions for visual content
- Custom icons for different step types
- Context-aware components using React Context API
- Fully typed with TypeScript
- Accessible with keyboard navigation support
- Responsive design that adapts to different screen sizes
- Smooth fade and slide animations for content transitions
- Composable architecture for flexible customization

## Props

### `<ChainOfThought />`

<PropertiesTable
  content={[
    {
      name: 'open',
      type: 'boolean',
      description:
        'Controlled open state of the collapsible.',
      isOptional: true,
    },
    {
      name: 'defaultOpen',
      type: 'boolean',
      description:
        'Default open state when uncontrolled. Defaults to false.',
      isOptional: true,
    },
    {
      name: 'onOpenChange',
      type: '(open: boolean) => void',
      description:
        'Callback when the open state changes.',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<"div">',
      description:
        'Any other props are spread to the root div element.',
      isOptional: true,
    },
  ]}
/>

### `<ChainOfThoughtHeader />`

<PropertiesTable
  content={[
    {
      name: 'children',
      type: 'React.ReactNode',
      description:
        'Custom header text. Defaults to "Chain of Thought".',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof CollapsibleTrigger>',
      description:
        'Any other props are spread to the CollapsibleTrigger component.',
      isOptional: true,
    },
  ]}
/>

### `<ChainOfThoughtStep />`

<PropertiesTable
  content={[
    {
      name: 'icon',
      type: 'LucideIcon',
      description:
        'Icon to display for the step. Defaults to DotIcon.',
      isOptional: true,
    },
    {
      name: 'label',
      type: 'string',
      description:
        'The main text label for the step.',
      isOptional: false,
    },
    {
      name: 'description',
      type: 'string',
      description:
        'Optional description text shown below the label.',
      isOptional: true,
    },
    {
      name: 'status',
      type: '"complete" | "active" | "pending"',
      description:
        'Visual status of the step. Defaults to "complete".',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<"div">',
      description:
        'Any other props are spread to the root div element.',
      isOptional: true,
    },
  ]}
/>

### `<ChainOfThoughtSearchResults />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<"div">',
      description:
        'Any props are spread to the container div element.',
      isOptional: true,
    },
  ]}
/>

### `<ChainOfThoughtSearchResult />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Badge>',
      description:
        'Any props are spread to the Badge component.',
      isOptional: true,
    },
  ]}
/>

### `<ChainOfThoughtContent />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof CollapsibleContent>',
      description:
        'Any props are spread to the CollapsibleContent component.',
      isOptional: true,
    },
  ]}
/>

### `<ChainOfThoughtImage />`

<PropertiesTable
  content={[
    {
      name: 'caption',
      type: 'string',
      description:
        'Optional caption text displayed below the image.',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<"div">',
      description:
        'Any other props are spread to the container div element.',
      isOptional: true,
    },
  ]}
/>