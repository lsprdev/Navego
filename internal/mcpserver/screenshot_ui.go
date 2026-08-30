package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const screenshotUIResourceURI = "ui://navego/screenshot-v2.html"

// AddUIResources registers every MCP Apps template referenced by Navego tools.
// Both browser workers and the public multi-browser control plane must expose
// these resources because clients resolve templates against the server that
// advertised the tool.
func AddUIResources(server *mcp.Server) {
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
    * { box-sizing: border-box; }
    [hidden] { display: none !important; }

    html, body {
      margin: 0;
      min-width: 0;
      background: transparent;
    }

    .capture {
      margin: 0;
      overflow: hidden;
      border-radius: 12px;
      background: #fff;
    }

    .capture__image {
      display: block;
      width: 100%;
      height: auto;
      max-height: 720px;
      object-fit: contain;
      background: #fff;
    }

    @media (prefers-reduced-motion: no-preference) {
      .capture__image { animation: reveal 140ms ease-out both; }
      @keyframes reveal {
        from { opacity: 0; }
        to { opacity: 1; transform: translateY(0); }
      }
    }
  </style>
</head>
<body>
  <figure class="capture" id="capture" hidden>
    <img class="capture__image" id="image" alt="Captura de tela feita pelo Navego" hidden>
  </figure>

  <script>
    (() => {
      const captureElement = document.getElementById("capture");
      const imageElement = document.getElementById("image");

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
        imageElement.onload = () => {
          imageElement.hidden = false;
          captureElement.hidden = false;
          notifyHeight();
        };
        imageElement.src = source;
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
