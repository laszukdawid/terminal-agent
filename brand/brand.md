# Terminal Agent Branding & UI Direction

## Product identity

Terminal Agent should not feel like a generic AI chat wrapper. It should feel like a **terminal-native agent control surface**: a local desktop interface for an assistant that understands the user’s shell, tools, environment, files, and workflows.

The core idea:

> **Your terminal. My context.**

Terminal Agent is not “chat with an AI model.”
It is a GUI for interacting with an agent that lives close to the terminal.

The product should feel:

* technical
* local-first
* keyboard-friendly
* shell-aware
* fast
* precise
* slightly playful, but not cute
* modern, but not SaaS-generic

It should avoid the standard AI-harness look: soft cards, purple gradients, sparkle icons, generic chat bubbles, oversized rounded input bars, and assistant-avatar layouts.

The visual identity should come from terminal metaphors:

```text
>_
$
~/
localhost
Ctrl + Enter
tokens
runtime
tools
env
history
copy
export
```

The product should feel like:

> **A desktop control panel for an AI agent that thinks in shell sessions.**

---

# Design north star

The winning direction is:

> **Terminal-native AI with a human-readable control surface.**

The app should not imitate a raw terminal exactly. It should be more polished, spacious, and approachable than a terminal emulator, while still being unmistakably inspired by command-line interaction.

The UI should answer this question visually:

> “What would an AI assistant look like if it were designed from the terminal outward, rather than from chat inward?”

---

# Layout structure

The UI has four main zones:

1. **Top application bar**
2. **Left navigation/sidebar**
3. **Main prompt and response workspace**
4. **Bottom agent identity / ambient status area**

The interface should be structured, calm, and operator-like.

It should feel more like a local development utility or system tool than a chatbot.

---

# Top application bar

## Left side

The top-left should contain a single terminal identity mark and the product name:

```text
>_ TERMINAL AGENT
```

Important: the terminal icon should appear only once here.
Do not duplicate the `>_` icon again at the top of the navigation sidebar.

The title should use uppercase monospace typography with slight letter spacing.

The icon and wordmark should feel like a terminal prompt, not like a generic app logo.

## Centre / right side

The model/runtime status should remain visible:

```text
● MODEL: codex / gpt-5.4-mini
```

This is an important part of the product identity. It communicates transparency and engineering usefulness.

Future variants could include:

```text
● MODEL: qwen3:14b
● LOCAL
● REMOTE
● TOOLS: 8
● CONTEXT: 24k
```

## Window controls

Keep only native/minimal window controls on the far right:

```text
—   □   ×
```

Remove the settings gear from the top bar.

Settings should live only in the left navigation panel. Having settings in both places creates ambiguity and visual duplication.

---

# Left sidebar

The sidebar should be a navigation and local-session control rail.

Suggested items:

```text
CHAT
HISTORY
ENV
TOOLS
SETTINGS
```

Each item can have a simple line icon:

* Chat bubble
* Clock/history
* Prompt/environment symbol
* Cube/toolbox
* Gear

The active item should have a subtle highlighted background and green text/icon.

The sidebar should feel like part of a terminal system UI, not a SaaS dashboard menu.

## Removed element

Do not place another large terminal logo at the top of the sidebar.
The product already has the `>_ TERMINAL AGENT` identity in the top bar.

The sidebar should begin directly with navigation or with subtle spacing.

---

# Voice/listening control

The voice control should live in the lower part of the sidebar, where the previous connection block was placed.

It should not be described as “Hold Space” or “press key to speak.”

The interaction model is:

> **Press to toggle microphone listening on/off.**

The control can look like a compact status card:

```text
[ mic icon ]

LISTEN
Press to toggle mic
```

or, when active:

```text
[ active mic icon ]

LISTENING
Press to stop
```

This area should be visually important but not overpower the main prompt.

The mic button should feel like a persistent mode toggle, not an action that submits anything.

Possible states:

```text
LISTEN
Press to toggle mic
```

```text
LISTENING
Press to stop
```

```text
MIC OFF
Press to listen
```

```text
NO MIC
Check settings
```

The button can use a circular mic icon with a green ring. In active mode, the ring could pulse subtly.

## Connection status

Connection status can still appear near this area, but should be secondary:

```text
● CONNECTED
localhost
~/
```

The old timestamp is optional. If space is tight, prioritize the listen control over showing the time.

---

# Main workspace

The main workspace should be the strongest part of the app.

It contains:

1. Prompt/input section
2. Response/output section
3. Copy/export actions below the response
4. Agent identity panel at the bottom

---

# Prompt section

The prompt section should be titled:

```text
ASK THE TERMINAL AGENT
```

The input field should look like a terminal prompt:

```text
> what model is this?
```

The prompt marker is part of the product identity and should remain visible.

The input should have:

* dark panel background
* thin green border
* monospace text
* generous height
* clear focus state
* inline keyboard hint
* one obvious action button inside the field

## Send button

Keep the `SEND` button inside the prompt field.

This placement is correct because it clearly indicates exactly what is being sent: the current prompt.

Example:

```text
> what model is this?             Ctrl + Enter to send     SEND >
```

The send button should be visually attached to the input field.

The bottom `SUBMIT` button should be removed entirely. It is redundant and unclear.

Possible button labels:

```text
SEND >
```

or:

```text
RUN >
```

`SEND` is clear.
`RUN` is more terminal-native.
Either can work, but do not have both.

---

# Response section

The response section should be titled:

```text
RESPONSE
```

The response should not look like a chat bubble. It should look like terminal output inside a structured panel.

Example:

```text
$ I’m an AI assistant running in your terminal environment.
$ I don’t have direct visibility into the exact
  underlying model name from here.

$ I can help you check it from the app/config
  or inspect available metadata.
```

The `$` markers reinforce that this is output from a terminal-agent session.

Continuation lines can be indented rather than prefixed repeatedly.

## Response metadata

Keep execution metadata at the bottom of the response panel:

```text
◷ 1.2s    |    ◌ 48 tokens    |    15:42:31
```

This makes the app feel observable and engineering-friendly.

Potential metadata:

* response time
* token count
* timestamp
* tool calls
* model used
* current working directory
* backend
* error status
* exit status

---

# Copy and export actions

Move `COPY` and `EXPORT` below the response panel.

They should no longer sit above or beside the response heading.

This makes the hierarchy better:

1. User reads the response
2. User decides what to do with it
3. Copy/export actions are available underneath

Suggested placement:

```text
[response panel]

COPY     |     EXPORT
```

The actions should be lightweight, line-icon based, and aligned under the response.

They should feel like terminal output actions, not document-toolbar buttons.

Possible labels:

```text
COPY
EXPORT
```

or:

```text
COPY OUTPUT
EXPORT LOG
```

The shorter labels are cleaner.

---

# Removed bottom controls

Remove the large bottom controls:

```text
LISTEN
Hold Space
```

```text
SUBMIT
Ctrl + Enter
```

Reason:

* Listen now lives in the sidebar as a mic toggle.
* Submit is redundant because Send lives inside the prompt field.
* The bottom area becomes cleaner and can better support the agent identity.

---

# Bottom agent identity area

The bottom area should focus on the ASCII/pixel agent identity.

This is a strong differentiator and should be preserved.

Suggested composition:

```text
[ASCII robot]   Your terminal.
                My context.

. . . . . . . . . . . . . . . . . . . . . . >_
```

This area creates personality without turning the product into a generic AI assistant.

The mascot should be:

* simple
* line-based
* monochrome or green
* terminal/pixel inspired
* not glossy
* not overly cute
* not childish

It should feel like a tiny process/avatar living inside the terminal session.

The phrase should remain:

```text
Your terminal.
My context.
```

It is concise and communicates the product promise well.

---

# Dark theme visual direction

The dark theme is the strongest brand expression.

It should feel like:

* modern terminal
* operator console
* local shell companion
* focused engineering tool
* slightly cybernetic
* calm and capable

It should not feel like:

* hacker movie parody
* neon cyberpunk toy
* Matrix cosplay
* generic AI chat app
* LM Studio clone

## Dark palette

Approximate direction:

```text
Background:       #050807 / #070B08
Panel:            #0A100C / #0D130F
Elevated panel:   #101812
Border:           #1D3A22 / #24542A
Accent green:     #66E05D / #70F064
Muted green:      #7BAF72
Primary text:     #E8E8DF
Secondary text:   #9BA39A
Disabled text:    #5E665E
```

Use near-black rather than pure black.

Use off-white rather than pure white.

Use green as the main accent, but avoid making every border bright green.

## Texture

A subtle dotted terminal texture works well.

The background may include:

* faint dot matrix
* subtle grid
* very low-opacity noise
* soft terminal phosphor feel

The texture must not harm readability.

The UI should feel tactile and system-like, not decorative.

## Borders and panels

Use thin bordered panels.

Prefer borders and subtle surface contrast over heavy shadows.

Panel edges should be crisp but not harsh.

Corners can be slightly rounded, but avoid pill-shaped SaaS styling.

---

# Light theme visual direction

The light theme should preserve the same terminal-native structure.

It should feel like:

* clean engineering workstation
* daylight terminal UI
* readable local tool
* calm and precise
* technical but welcoming

It should not become:

* generic white AI chat
* card-based SaaS dashboard
* pastel assistant UI
* Claude/ChatGPT-style desktop clone

## Light palette

Approximate direction:

```text
Background:       #FAFAF7 / #F8F9F4
Panel:            #FFFFFF / #FCFCFA
Subtle panel:     #F4F7F0
Border:           #DDE5D8
Green border:     #9CCB91
Accent green:     #2E8B2E / #278327
Dark green:       #17691E
Primary text:     #171A17
Secondary text:   #5F665F
Muted text:        #8A918A
```

The light theme should keep:

* monospace typography
* `>_` prompt identity
* `$` response markers
* left navigation
* sidebar listen toggle
* runtime model status
* response metadata
* bottom ASCII agent area

---

# Typography

Typography is central to the brand.

Use a high-quality monospace font for most core UI elements.

Good candidates:

* JetBrains Mono
* Berkeley Mono
* IBM Plex Mono
* Commit Mono
* Geist Mono
* Iosevka

Use monospace for:

* product name
* nav labels
* prompt input
* response output
* metadata
* keyboard shortcuts
* button labels
* model status

The UI should feel like a crafted terminal interface, not like a normal app with monospace sprinkled on top.

Suggested hierarchy:

```text
Product title:     uppercase monospace, tracked
Section labels:    uppercase monospace, green
Prompt text:       large monospace
Response text:     readable monospace, generous line height
Metadata:          small monospace, muted
Buttons:           monospace, command-like
```

---

# Interaction principles

## Keyboard-first

The app should feel optimized for fast keyboard use.

Keep hints like:

```text
Ctrl + Enter to send
Esc to cancel
Tab to complete
↑ previous prompt
```

But do not overload the UI with shortcuts everywhere.

## Prompt-first

The prompt is the central interaction point.

The user should understand:

* where to type
* what will be sent
* how to send it
* what context the agent has

## Observable execution

The app should expose runtime behavior where useful.

Examples:

```text
running...
tool: inspect_config
status: complete
1.2s
48 tokens
```

This makes the app trustworthy for technical users.

## Agent as local process

The agent should feel like something running locally or near-locally, not like a distant SaaS chatbot.

Useful concepts:

```text
localhost
~/
env
tools
history
model
tokens
runtime
```

---

# Component primitives

Useful reusable components:

```text
AppShell
TopBar
SidebarNav
SidebarListenToggle
ModelStatusPill
PromptPanel
PromptInput
SendButton
OutputPanel
OutputMetadata
OutputActions
AgentMascotPanel
CommandButton
StatusBadge
```

Design these as a coherent system rather than separate widgets.

---

# Final updated structure

The intended dark-theme layout is:

```text
┌──────────────────────────────────────────────────────────────┐
│ >_ TERMINAL AGENT          ● MODEL: codex / gpt-5.4-mini   — □ × │
├───────────────┬──────────────────────────────────────────────┤
│ CHAT          │ ASK THE TERMINAL AGENT                       │
│ HISTORY       │ ┌──────────────────────────────────────────┐ │
│ ENV           │ │ > what model is this?   Ctrl+Enter  SEND │ │
│ TOOLS         │ └──────────────────────────────────────────┘ │
│ SETTINGS      │                                              │
│               │ RESPONSE                                     │
│               │ ┌──────────────────────────────────────────┐ │
│               │ │ $ response text...                       │ │
│ [mic toggle]  │ │                                          │ │
│ LISTEN        │ │ ◷ 1.2s | 48 tokens | 15:42:31             │ │
│ Press toggle  │ └──────────────────────────────────────────┘ │
│               │ COPY     |     EXPORT                        │
│ ● CONNECTED   │                                              │
│ localhost     │ ┌──────────────────────────────────────────┐ │
│ ~/            │ │ ASCII agent: Your terminal. My context.  │ │
│               │ └──────────────────────────────────────────┘ │
└───────────────┴──────────────────────────────────────────────┘
```

This structure removes the duplicate settings, duplicate terminal icon, redundant submit button, and misplaced listen control.

It keeps the strongest parts:

```text
>_ identity
terminal prompt input
single clear SEND action
sidebar navigation
sidebar mic toggle
shell-like response output
copy/export under response
runtime metadata
ASCII agent personality
dark terminal-native atmosphere
```

The overall direction is strong. The next design pass should refine spacing, typography, exact icon style, active/listening states, and component consistency rather than changing the core concept.

