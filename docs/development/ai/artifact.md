
# Artifact

The `Artifact` component provides a structured container for displaying generated content like code, documents, or other outputs with built-in header actions.

<Preview path="artifact" />

## Installation

```sh
npx ai-elements@latest add artifact
```

## Usage

```tsx
import {
  Artifact,
  ArtifactAction,
  ArtifactActions,
  ArtifactContent,
  ArtifactDescription,
  ArtifactHeader,
  ArtifactTitle,
} from '@/components/ai-elements/artifact';
```

```tsx
<Artifact>
  <ArtifactHeader>
    <div>
      <ArtifactTitle>Dijkstra's Algorithm Implementation</ArtifactTitle>
      <ArtifactDescription>Updated 1 minute ago</ArtifactDescription>
    </div>
    <ArtifactActions>
      <ArtifactAction icon={CopyIcon} label="Copy" tooltip="Copy to clipboard" />
    </ArtifactActions>
  </ArtifactHeader>
  <ArtifactContent>
    {/* Your content here */}
  </ArtifactContent>
</Artifact>
```

## Features

- Structured container with header and content areas
- Built-in header with title and description support
- Flexible action buttons with tooltips
- Customizable styling for all subcomponents
- Support for close buttons and action groups
- Clean, modern design with border and shadow
- Responsive layout that adapts to content
- TypeScript support with proper type definitions
- Composable architecture for maximum flexibility

## Examples

### With Code Display

<Preview path="artifact" />

## Props

### `<Artifact />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description:
        'Any other props are spread to the underlying div element.',
      isOptional: true,
    },
  ]}
/>

### `<ArtifactHeader />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description:
        'Any other props are spread to the underlying div element.',
      isOptional: true,
    },
  ]}
/>

### `<ArtifactTitle />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLParagraphElement>',
      description:
        'Any other props are spread to the underlying paragraph element.',
      isOptional: true,
    },
  ]}
/>

### `<ArtifactDescription />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLParagraphElement>',
      description:
        'Any other props are spread to the underlying paragraph element.',
      isOptional: true,
    },
  ]}
/>

### `<ArtifactActions />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description:
        'Any other props are spread to the underlying div element.',
      isOptional: true,
    },
  ]}
/>

### `<ArtifactAction />`

<PropertiesTable
  content={[
    {
      name: 'tooltip',
      type: 'string',
      description: 'Tooltip text to display on hover.',
      isOptional: true,
    },
    {
      name: 'label',
      type: 'string',
      description: 'Screen reader label for the action button.',
      isOptional: true,
    },
    {
      name: 'icon',
      type: 'LucideIcon',
      description: 'Lucide icon component to display in the button.',
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

### `<ArtifactClose />`

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

### `<ArtifactContent />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.HTMLAttributes<HTMLDivElement>',
      description:
        'Any other props are spread to the underlying div element.',
      isOptional: true,
    },
  ]}
/>