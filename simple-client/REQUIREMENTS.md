# Simple Client -- Requirements

## Overview

A lightweight web client for asking questions and receiving answers from the MCP Authenticated Server. The client is a static TypeScript/HTML application served by Caddy, configured via a TOML file, and brandable via CSS and logo. The emphasis is on simplicity: one text box to ask, one box to read the answer, and a history slider to revisit prior exchanges.

## Architecture

```
Browser (static HTML/TS/CSS)
    |
    | HTTPS
    v
Caddy (web server + reverse proxy)
    |
    | /api/* --> proxy to MCP server
    v
MCP Authenticated Server (POST /mcp)
```

The client consists of:

1. **Caddy** -- serves static files and reverse-proxies API requests to the MCP server.
2. **Static frontend** -- TypeScript compiled to JavaScript, HTML, and CSS. No framework required. Bundled at build time.
3. **TOML config file** -- provides MCP server URL, auth credentials, branding, and client behavior settings.

## Requirements

### CFG -- Configuration

**CFG-01**: The client reads a TOML configuration file at startup. Default path: `config.toml` in the working directory.

**CFG-02**: Configuration fields:

```toml
[server]
# MCP server base URL (proxied through Caddy)
mcp_url = "http://localhost:8080"
# MCP tool name to call for search
tool_name = "search_documents"
# Maximum number of results to request per search
result_limit = 5

[auth]
# AWS Cognito region
region = "us-east-1"
# Cognito User Pool ID
user_pool_id = "us-east-1_EXAMPLE"
# Cognito App Client ID
client_id = "EXAMPLE_CLIENT_ID"

[branding]
# Application title shown in the header and browser tab
title = "Knowledge Base"
# Path to logo image file (relative to static root)
logo = ""
# Path to custom CSS file (relative to static root)
custom_css = ""

[client]
# Maximum number of Q&A pairs to retain in history
max_history = 100
# Placeholder text shown in the question input
placeholder = "Ask a question..."
```

**CFG-03**: Caddy reads the TOML to configure the reverse proxy target (`mcp_url`) and static file root. A Caddyfile template or generation script produces the final Caddyfile from the TOML values.

**CFG-04**: The frontend reads a subset of the config (branding, client settings) injected at build time or served as a JSON endpoint (`/config.json`) generated from the TOML.

### AUTH -- Authentication

**AUTH-01**: The client supports username/password authentication against AWS Cognito. The login form collects username and password and exchanges them for tokens via the Cognito `InitiateAuth` API (USER_PASSWORD_AUTH flow).

**AUTH-02**: On successful authentication, the client stores the access token, refresh token, and expiration time in memory (not localStorage, not cookies).

**AUTH-03**: The access token is included as a `Bearer` token in the `Authorization` header on all proxied requests to the MCP server.

**AUTH-04**: When the access token expires, the client automatically refreshes it using the refresh token before retrying the request. If refresh fails, the client returns to the login screen.

**AUTH-05**: The login screen is shown on initial load if no valid token exists. The login screen respects branding (logo, title, custom CSS).

**AUTH-06**: A logout button clears all tokens and returns to the login screen.

### UI -- User Interface

**UI-01**: The page has three areas:
1. **Header** -- logo (if configured), title, and logout button.
2. **Main area** -- question input and answer display, stacked vertically. The question input is at the top, the answer display is below it.
3. **History slider** -- a vertical slider on the left side that navigates between prior Q&A pairs.

**UI-02**: The question input is a multi-line text box. The user types a question and submits by clicking a Send button or pressing Ctrl+Enter. The text box is resizable vertically.

**UI-03**: The answer display is a read-only area below the question input. It renders the answer as plain text or markdown. Answers that exceed the visible area have a vertical scrollbar.

**UI-04**: Questions that exceed the visible area of the question input have a vertical scrollbar. The input does not grow unbounded.

**UI-05**: While a request is in flight, the Send button is disabled and a loading indicator is visible in the answer area. The question input remains visible with the submitted text.

**UI-06**: On error (network failure, auth failure, MCP error), the answer area displays the error message. Errors are visually distinct from answers (different background or border color).

### HIST -- History

**HIST-01**: Each submitted question and its corresponding answer form a **pair**. The pair is the atomic unit of history.

**HIST-02**: The history slider on the left side is a vertical control that lets the user navigate between pairs. The most recent pair is at the bottom, the oldest at the top.

**HIST-03**: Selecting a history entry loads both the question and the answer into their respective areas. The question input shows the historical question (read-only or editable -- see HIST-06). The answer area shows the historical answer.

**HIST-04**: The currently active pair is visually highlighted in the history slider. Each entry in the slider shows a truncated preview of the question text (first 60 characters).

**HIST-05**: The history slider supports scrolling if there are more entries than fit in the visible area.

**HIST-06**: Selecting a historical pair loads the question into the input as editable text. The user can modify it and resubmit, which creates a new pair (does not overwrite the old one).

**HIST-07**: History is stored in the browser's sessionStorage. It persists across page refreshes within the same tab but is cleared when the tab is closed.

**HIST-08**: History respects `max_history` from config. When the limit is reached, the oldest pair is removed.

**HIST-09**: A "New Question" button (or equivalent) clears the question input and answer area, positioning the user to ask a fresh question. This does not delete history.

### MCP -- MCP Protocol Integration

**MCP-01**: The client implements the MCP Streamable HTTP protocol. On first use after authentication, the client sends an `initialize` request, captures the `Mcp-Session-Id` header, and sends a `notifications/initialized` notification.

**MCP-02**: Subsequent `tools/call` requests include the `Mcp-Session-Id` and `Mcp-Protocol-Version` headers.

**MCP-03**: The client calls the configured tool (default: `search_documents`) with `{"query": "<user question>", "limit": <result_limit>}`.

**MCP-04**: The client parses the SSE response format from the Streamable HTTP transport. It extracts JSON from `data:` lines in the event stream.

**MCP-05**: The client extracts the `result.content[].text` fields from the JSON-RPC response and concatenates them as the answer text.

**MCP-06**: If the MCP response contains an error (JSON-RPC error, guardrail rejection), the error message is displayed in the answer area per UI-06.

**MCP-07**: If the MCP session becomes invalid (server restart, session timeout), the client re-initializes the session transparently before retrying the request.

### BRAND -- Branding

**BRAND-01**: The default styling uses a neutral color palette with no logos. It looks professional without customization.

**BRAND-02**: A custom CSS file (configured via `branding.custom_css`) is loaded after the default styles, allowing full override of any visual element.

**BRAND-03**: A logo image (configured via `branding.logo`) is displayed in the header. If no logo is configured, the header shows only the title text.

**BRAND-04**: The application title (configured via `branding.title`) appears in the header and as the browser tab title (`<title>` element).

**BRAND-05**: The CSS class names and HTML structure are stable and documented so that custom CSS can target specific elements reliably. Key classes:
- `.sc-header` -- header bar
- `.sc-logo` -- logo image
- `.sc-title` -- title text
- `.sc-history` -- history slider panel
- `.sc-history-item` -- individual history entry
- `.sc-history-item--active` -- currently selected entry
- `.sc-question` -- question input area
- `.sc-answer` -- answer display area
- `.sc-answer--error` -- answer area when showing an error
- `.sc-send` -- send button
- `.sc-loading` -- loading indicator
- `.sc-login` -- login form container

### SERVE -- Serving

**SERVE-01**: Caddy serves the static frontend files (HTML, JS, CSS, images) from a configured root directory.

**SERVE-02**: Caddy reverse-proxies requests under `/api/mcp` to the MCP server URL from the TOML config. The proxy passes through all headers including `Authorization`, `Mcp-Session-Id`, and `Mcp-Protocol-Version`.

**SERVE-03**: Caddy handles TLS automatically via its built-in ACME support (for production) or serves plain HTTP for local development.

**SERVE-04**: A Makefile provides:
- `make build` -- compile TypeScript, bundle assets
- `make run` -- start Caddy with the generated Caddyfile
- `make dev` -- build + run with file watching for development
- `make clean` -- remove build artifacts

**SERVE-05**: The Caddyfile is generated from the TOML config via a script or template. It is not manually maintained.

### LAYOUT -- Responsive Layout

**LAYOUT-01**: The history slider occupies a fixed-width column on the left (default: 280px). It collapses to a hamburger menu on viewports narrower than 768px.

**LAYOUT-02**: The main area (question + answer) fills the remaining width.

**LAYOUT-03**: The question input occupies approximately 25% of the main area height. The answer area occupies the remaining 75%. Both areas have minimum heights to remain usable.

**LAYOUT-04**: On mobile viewports (< 768px), the layout stacks vertically: history (collapsed), question, answer. The history panel slides in from the left when the hamburger menu is tapped.

## Non-Requirements

- **No chat history on the server.** All history is client-side only.
- **No multi-turn conversation.** Each question is independent. The client does not send prior Q&A pairs as context.
- **No user management.** The client authenticates against Cognito but does not manage users, passwords, or groups.
- **No offline support.** The client requires network access to the MCP server and Cognito.
- **No streaming answers.** The client waits for the complete MCP response before displaying the answer.

## File Structure

```
simple-client/
  REQUIREMENTS.md          # This file
  config.toml.example      # Example configuration
  Caddyfile.template       # Caddy config template
  Makefile                 # Build and run targets
  src/
    index.html             # Main page
    main.ts                # Entry point
    mcp.ts                 # MCP protocol client (initialize, tools/call, SSE parsing)
    auth.ts                # Cognito authentication (login, token refresh)
    history.ts             # Q&A pair storage and navigation
    ui.ts                  # DOM manipulation and event binding
  static/
    style.css              # Default styles
    custom.css             # Placeholder for branding overrides
  scripts/
    generate-caddyfile.sh  # TOML -> Caddyfile generator
    generate-config.sh     # TOML -> /config.json generator
```
