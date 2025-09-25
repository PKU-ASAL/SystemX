
# Tool

The `Tool` component displays a collapsible interface for showing/hiding tool details. It is designed to take the `ToolUIPart` type from the AI SDK and display it in a collapsible interface.

<Preview path="tool" />

## Installation

```sh
npx ai-elements@latest add tool
```

## Usage

```tsx
import {
  Tool,
  ToolContent,
  ToolHeader,
  ToolOutput,
  ToolInput,
} from '@/components/ai-elements/tool';
```

```tsx
<Tool>
  <ToolHeader type="tool-call" state={'output-available' as const} />
  <ToolContent>
    <ToolInput input="Input to tool call" />
    <ToolOutput errorText="Error" output="Output from tool call" />
  </ToolContent>
</Tool>
```

## Usage in AI SDK

Build a simple stateful weather app that renders the last message in a tool using [`useChat`](/docs/reference/ai-sdk-ui/use-chat).

Add the following component to your frontend:

```tsx filename="app/page.tsx"
'use client';

import { useChat } from '@ai-sdk/react';
import { DefaultChatTransport, type ToolUIPart } from 'ai';
import { Button } from '@/components/ui/button';
import { Response } from '@/components/ai-elements/response';
import {
  Tool,
  ToolContent,
  ToolHeader,
  ToolInput,
  ToolOutput,
} from '@/components/ai-elements/tool';

type WeatherToolInput = {
  location: string;
  units: 'celsius' | 'fahrenheit';
};

type WeatherToolOutput = {
  location: string;
  temperature: string;
  conditions: string;
  humidity: string;
  windSpeed: string;
  lastUpdated: string;
};

type WeatherToolUIPart = ToolUIPart<{
  fetch_weather_data: {
    input: WeatherToolInput;
    output: WeatherToolOutput;
  };
}>;

const Example = () => {
  const { messages, sendMessage, status } = useChat({
    transport: new DefaultChatTransport({
      api: '/api/weather',
    }),
  });

  const handleWeatherClick = () => {
    sendMessage({ text: 'Get weather data for San Francisco in fahrenheit' });
  };

  const latestMessage = messages[messages.length - 1];
  const weatherTool = latestMessage?.parts?.find(
    (part) => part.type === 'tool-fetch_weather_data',
  ) as WeatherToolUIPart | undefined;

  return (
    <div className="max-w-4xl mx-auto p-6 relative size-full rounded-lg border h-[600px]">
      <div className="flex flex-col h-full">
        <div className="space-y-4">
          <Button onClick={handleWeatherClick} disabled={status !== 'ready'}>
            Get Weather for San Francisco
          </Button>

          {weatherTool && (
            <Tool defaultOpen={true}>
              <ToolHeader type="tool-fetch_weather_data" state={weatherTool.state} />
              <ToolContent>
                <ToolInput input={weatherTool.input} />
                <ToolOutput
                  output={
                    <Response>
                      {formatWeatherResult(weatherTool.output)}
                    </Response>
                  }
                  errorText={weatherTool.errorText}
                />
              </ToolContent>
            </Tool>
          )}
        </div>
      </div>
    </div>
  );
};

function formatWeatherResult(result: WeatherToolOutput): string {
  return `**Weather for ${result.location}**

**Temperature:** ${result.temperature}  
**Conditions:** ${result.conditions}  
**Humidity:** ${result.humidity}  
**Wind Speed:** ${result.windSpeed}  

*Last updated: ${result.lastUpdated}*`;
}

export default Example;
```

Add the following route to your backend:

```ts filename="app/api/weather/route.tsx"
import { streamText, UIMessage, convertToModelMessages } from 'ai';
import { z } from 'zod';

// Allow streaming responses up to 30 seconds
export const maxDuration = 30;

export async function POST(req: Request) {
  const { messages }: { messages: UIMessage[] } = await req.json();

  const result = streamText({
    model: 'openai/gpt-4o',
    messages: convertToModelMessages(messages),
    tools: {
      fetch_weather_data: {
        description: 'Fetch weather information for a specific location',
        parameters: z.object({
          location: z
            .string()
            .describe('The city or location to get weather for'),
          units: z
            .enum(['celsius', 'fahrenheit'])
            .default('celsius')
            .describe('Temperature units'),
        }),
        inputSchema: z.object({
          location: z.string(),
          units: z.enum(['celsius', 'fahrenheit']).default('celsius'),
        }),
        execute: async ({ location, units }) => {
          await new Promise((resolve) => setTimeout(resolve, 1500));

          const temp =
            units === 'celsius'
              ? Math.floor(Math.random() * 35) + 5
              : Math.floor(Math.random() * 63) + 41;

          return {
            location,
            temperature: `${temp}°${units === 'celsius' ? 'C' : 'F'}`,
            conditions: 'Sunny',
            humidity: `12%`,
            windSpeed: `35 ${units === 'celsius' ? 'km/h' : 'mph'}`,
            lastUpdated: new Date().toLocaleString(),
          };
        },
      },
    },
  });

  return result.toUIMessageStreamResponse();
}
```

## Features

- Collapsible interface for showing/hiding tool details
- Visual status indicators with icons and badges
- Support for multiple tool execution states (pending, running, completed, error)
- Formatted parameter display with JSON syntax highlighting
- Result and error handling with appropriate styling
- Composable structure for flexible layouts
- Accessible keyboard navigation and screen reader support
- Consistent styling that matches your design system
- Auto-opens completed tools by default for better UX

## Examples

### Input Streaming (Pending)

Shows a tool in its initial state while parameters are being processed.

<Preview path="tool-input-streaming" />

### Input Available (Running)

Shows a tool that's actively executing with its parameters.

<Preview path="tool-input-available" />

### Output Available (Completed)

Shows a completed tool with successful results. Opens by default to show the results. In this instance, the output is a JSON object, so we can use the `CodeBlock` component to display it.

<Preview path="tool-output-available" />

### Output Error

Shows a tool that encountered an error during execution. Opens by default to display the error.

<Preview path="tool-output-error" />

## Props

### `<Tool />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof Collapsible>',
      description:
        'Any other props are spread to the root Collapsible component.',
      isOptional: true,
    },
  ]}
/>

### `<ToolHeader />`

<PropertiesTable
  content={[
    {
      name: 'type',
      type: 'ToolUIPart["type"]',
      description: 'The type/name of the tool.',
      isOptional: false,
    },
    {
      name: 'state',
      type: 'ToolUIPart["state"]',
      description:
        'The current state of the tool (input-streaming, input-available, output-available, or output-error).',
      isOptional: false,
    },
    {
      name: 'className',
      type: 'string',
      description: 'Additional CSS classes to apply to the header.',
      isOptional: true,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof CollapsibleTrigger>',
      description: 'Any other props are spread to the CollapsibleTrigger.',
      isOptional: true,
    },
  ]}
/>

### `<ToolContent />`

<PropertiesTable
  content={[
    {
      name: '[...props]',
      type: 'React.ComponentProps<typeof CollapsibleContent>',
      description: 'Any other props are spread to the CollapsibleContent.',
      isOptional: true,
    },
  ]}
/>

### `<ToolInput />`

<PropertiesTable
  content={[
    {
      name: 'input',
      type: 'ToolUIPart["input"]',
      description:
        'The input parameters passed to the tool, displayed as formatted JSON.',
      isOptional: false,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<"div">',
      description: 'Any other props are spread to the underlying div.',
      isOptional: true,
    },
  ]}
/>

### `<ToolOutput />`

<PropertiesTable
  content={[
    {
      name: 'output',
      type: 'React.ReactNode',
      description: 'The output/result of the tool execution.',
      isOptional: false,
    },
    {
      name: 'errorText',
      type: 'ToolUIPart["errorText"]',
      description: 'An error message if the tool execution failed.',
      isOptional: false,
    },
    {
      name: '[...props]',
      type: 'React.ComponentProps<"div">',
      description: 'Any other props are spread to the underlying div.',
      isOptional: true,
    },
  ]}
/>
