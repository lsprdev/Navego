package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const screenshotUIResourceURI = "ui://navego/screenshot.html"

func addScreenshotUIResource(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         screenshotUIResourceURI,
		Name:        "navego-screenshot",
		Title:       "Navego screenshot",
		Description: "Inline viewer for screenshots captured by Navego.",
		MIMEType:    "text/html;profile=mcp-app",
		Meta:        screenshotUIResourceMeta(),
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      screenshotUIResourceURI,
			MIMEType: "text/html;profile=mcp-app",
			Text:     screenshotUIHTML,
			Meta:     screenshotUIResourceMeta(),
		}}}, nil
	})
}

func screenshotUIResourceMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"prefersBorder": true,
		},
	}
}

const screenshotUIHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light dark">
  <style>
    :root {
      color-scheme: light dark;
      --paper: light-dark(#f5f1e8, #141515);
      --ink: light-dark(#191a17, #f4f0e7);
      --muted: light-dark(#686b62, #a7aaa1);
      --line: light-dark(#d3cec2, #343735);
      --accent: #ff6b35;
    }

    * { box-sizing: border-box; }

    html, body {
      margin: 0;
      min-width: 0;
      background: transparent;
      color: var(--ink);
      font-family: Georgia, "Times New Roman", serif;
    }

    .capture {
      margin: 0;
      overflow: hidden;
      border: 1px solid var(--line);
      border-radius: 14px;
      background: var(--paper);
    }

    .capture__bar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      min-height: 42px;
      padding: 9px 12px;
      border-bottom: 1px solid var(--line);
      font-family: ui-monospace, "SFMono-Regular", Consolas, monospace;
      font-size: 11px;
      letter-spacing: .12em;
      text-transform: uppercase;
    }

    .capture__brand {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      font-weight: 700;
    }

    .capture__brand::before {
      width: 8px;
      height: 8px;
      border-radius: 2px;
      background: var(--accent);
      box-shadow: 5px 5px 0 color-mix(in srgb, var(--accent) 45%, transparent);
      content: "";
    }

    .capture__meta { color: var(--muted); }

    .capture__empty {
      display: grid;
      min-height: 128px;
      place-items: center;
      padding: 24px;
      color: var(--muted);
      font-style: italic;
      text-align: center;
    }

    .capture__image {
      display: none;
      width: 100%;
      height: auto;
      max-height: 720px;
      object-fit: contain;
      background: #fff;
    }

    .capture__actions {
      display: flex;
      justify-content: flex-end;
      padding: 8px 12px;
      border-top: 1px solid var(--line);
    }

    .capture__download {
      color: var(--ink);
      font-family: ui-monospace, "SFMono-Regular", Consolas, monospace;
      font-size: 11px;
      letter-spacing: .06em;
      text-underline-offset: 3px;
    }

    @media (prefers-reduced-motion: no-preference) {
      .capture__image { animation: reveal 180ms ease-out both; }
      @keyframes reveal {
        from { opacity: 0; transform: translateY(3px); }
        to { opacity: 1; transform: translateY(0); }
      }
    }
  </style>
</head>
<body>
  <figure class="capture" aria-live="polite">
    <figcaption class="capture__bar">
      <span class="capture__brand">Navego</span>
      <span class="capture__meta" id="meta">captura</span>
    </figcaption>
    <div class="capture__empty" id="empty">Preparando a captura…</div>
    <img class="capture__image" id="image" alt="Captura de tela feita pelo Navego">
    <div class="capture__actions" id="actions" hidden>
      <a class="capture__download" id="download" download="navego-screenshot.png">Baixar PNG</a>
    </div>
  </figure>

  <script>
    (() => {
      const imageElement = document.getElementById("image");
      const emptyElement = document.getElementById("empty");
      const actionsElement = document.getElementById("actions");
      const downloadElement = document.getElementById("download");
      const metaElement = document.getElementById("meta");

      function findImage(value, seen = new Set(), depth = 0) {
        if (!value || typeof value !== "object" || depth > 12 || seen.has(value)) return null;
        seen.add(value);
        if (
          value.type === "image" &&
          typeof value.data === "string" &&
          typeof (value.mimeType || value.mime_type) === "string"
        ) return value;
        if (Array.isArray(value)) {
          for (const item of value) {
            const found = findImage(item, seen, depth + 1);
            if (found) return found;
          }
          return null;
        }
        for (const item of Object.values(value)) {
          const found = findImage(item, seen, depth + 1);
          if (found) return found;
        }
        return null;
      }

      function notifyHeight() {
        window.openai?.notifyIntrinsicHeight?.();
      }

      function render(value) {
        const image = findImage(value);
        if (!image) return false;
        const mimeType = image.mimeType || image.mime_type || "image/png";
        const source = "data:" + mimeType + ";base64," + image.data;
        imageElement.onload = notifyHeight;
        imageElement.src = source;
        downloadElement.href = source;
        downloadElement.download = mimeType === "image/jpeg" ? "navego-screenshot.jpg" : "navego-screenshot.png";
        metaElement.textContent = mimeType.replace("image/", "");
        emptyElement.hidden = true;
        imageElement.style.display = "block";
        actionsElement.hidden = false;
        notifyHeight();
        return true;
      }

      function renderFromHost() {
        const host = window.openai;
        return render(host?.toolResponseMetadata) || render(host?.toolOutput);
      }

      window.addEventListener("openai:set_globals", (event) => {
        const globals = event.detail?.globals;
        render(globals?.toolResponseMetadata) || render(globals?.toolOutput);
      }, { passive: true });

      window.addEventListener("message", (event) => {
        if (event.source !== window.parent) return;
        const message = event.data;
        if (message?.method === "ui/notifications/tool-result") render(message.params);
      }, { passive: true });

      [0, 50, 200, 750].forEach((delay) => setTimeout(renderFromHost, delay));
    })();
  </script>
</body>
</html>`
