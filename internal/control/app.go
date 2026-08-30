package control

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/lsprdev/Navego/pb_migrations"
)

const maxBrowsersPerUser = 5

type Config struct {
	DataDir                string
	AgentToken             string
	WorkerAPIKey           string
	PublicViewerURL        string
	PublicDashboardOrigins string
	VaultKey               string
	InternalHTTP           *http.Client
}

func New(cfg Config) *pocketbase.PocketBase {
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  cfg.DataDir,
		HideStartBanner: true,
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{})
	app.OnBootstrap().BindFunc(func(event *core.BootstrapEvent) error {
		if err := event.Next(); err != nil {
			return err
		}
		return event.App.RunAppMigrations()
	})
	if cfg.InternalHTTP == nil {
		cfg.InternalHTTP = &http.Client{Timeout: 20 * time.Second}
	}
	viewerAccess := newViewerStore()
	app.OnServe().BindFunc(func(event *core.ServeEvent) error {
		// Navego never exposes PocketBase's browser installer. Administrators are
		// created explicitly with the CLI, which avoids logging a short-lived
		// superuser URL in container logs.
		event.InstallerFunc = nil
		registerRoutes(event, cfg, viewerAccess)
		return event.Next()
	})

	return app
}

func registerRoutes(event *core.ServeEvent, cfg Config, viewerAccess *viewerStore) {
	vault, vaultErr := newCredentialVault(cfg.VaultKey)
	event.Router.GET("/api/navego/healthz", func(request *core.RequestEvent) error {
		return request.JSON(http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "navego-control",
		})
	})

	protected := event.Router.Group("/api/navego").Bind(apis.RequireAuth("users"))
	protected.GET("/browsers", listBrowsers)
	protected.GET("/activity", listActivity)
	protected.GET("/credentials", listCredentials(vault, vaultErr))
	protected.POST("/credentials", createCredential(vault, vaultErr))
	protected.PATCH("/credentials/{id}", updateCredential(vault, vaultErr))
	protected.DELETE("/credentials/{id}", deleteCredential(vault, vaultErr))
	protected.POST("/browsers", createBrowser)
	protected.PATCH("/browsers/{id}", renameBrowser)
	protected.POST("/browsers/{id}/power", powerBrowser)
	protected.GET("/browsers/{id}/preview", previewBrowser(cfg.WorkerAPIKey, cfg.InternalHTTP))
	protected.POST("/browsers/{id}/viewer-ticket", mintViewerTicket(viewerAccess, cfg.PublicViewerURL))
	protected.DELETE("/browsers/{id}", deleteBrowser)

	publicViewer, _ := validatedPublicViewerURL(cfg.PublicViewerURL)
	event.Router.GET("/viewer/session/{ticket}", beginViewerSession(viewerAccess, strings.HasPrefix(publicViewer, "https://"), cfg.PublicDashboardOrigins))
	event.Router.Any("/viewer/{path...}", proxyViewer(viewerAccess, true, cfg.PublicDashboardOrigins))
	event.Router.Any("/switch", proxyViewer(viewerAccess, false, cfg.PublicDashboardOrigins))

	if agentToken := strings.TrimSpace(cfg.AgentToken); agentToken != "" {
		internal := event.Router.Group("/api/navego/internal").BindFunc(requireAgentToken(agentToken))
		internal.GET("/commands", listAgentCommands)
		internal.PATCH("/browsers/{id}", reportBrowserState)
		internal.DELETE("/browsers/{id}", confirmBrowserDeletion)
	}
}

func previewBrowser(workerAPIKey string, client *http.Client) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		record, apiErr := ownedBrowser(event)
		if apiErr != nil {
			return apiErr
		}
		if record.GetString("state") != "running" {
			return apis.NewApiError(http.StatusConflict, "O navegador ainda não está disponível.", nil)
		}
		endpoint, err := validatedWorkerEndpoint(record.GetString("worker_endpoint"))
		if err != nil {
			return event.InternalServerError("Endpoint interno do navegador inválido.", err)
		}
		request, err := http.NewRequestWithContext(event.Request.Context(), http.MethodGet, endpoint+"/internal/screenshot", nil)
		if err != nil {
			return event.InternalServerError("Não foi possível preparar a captura.", err)
		}
		if key := strings.TrimSpace(workerAPIKey); key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
		response, err := client.Do(request)
		if err != nil {
			return event.InternalServerError("O worker do navegador não respondeu.", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return event.InternalServerError("O worker não conseguiu capturar a tela.", fmt.Errorf("worker returned %s", response.Status))
		}
		const maxScreenshotBytes = 12 << 20
		data, err := io.ReadAll(io.LimitReader(response.Body, maxScreenshotBytes+1))
		if err != nil || len(data) > maxScreenshotBytes {
			return event.InternalServerError("A captura recebida é inválida.", err)
		}
		mimeType := response.Header.Get("Content-Type")
		if mimeType != "image/png" && mimeType != "image/jpeg" {
			return event.InternalServerError("O worker retornou um formato de captura inválido.", nil)
		}
		event.Response.Header().Set("Cache-Control", "private, no-store")
		event.Response.Header().Set("X-Content-Type-Options", "nosniff")
		return event.Blob(http.StatusOK, mimeType, data)
	}
}

func validatedWorkerEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("malformed worker endpoint")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if !strings.HasPrefix(hostname, "navego-browser-") || parsed.Port() != "8001" {
		return "", fmt.Errorf("worker endpoint is outside the runtime network")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

type browserResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

type agentBrowser struct {
	ID               string `json:"id"`
	OwnerID          string `json:"owner_id"`
	Name             string `json:"name"`
	State            string `json:"state"`
	BrowserContainer string `json:"browser_container"`
	WorkerContainer  string `json:"worker_container"`
	ProfileVolume    string `json:"profile_volume"`
}

type activityResponse struct {
	ID          string `json:"id"`
	Event       string `json:"event"`
	Result      string `json:"result"`
	BrowserID   string `json:"browser_id,omitempty"`
	BrowserName string `json:"browser_name,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func listActivity(event *core.RequestEvent) error {
	records, err := event.App.FindRecordsByFilter(
		pb_migrations.AuditEventsCollection,
		"owner = {:owner}",
		"-created",
		50,
		0,
		dbx.Params{"owner": event.Auth.Id},
	)
	if err != nil {
		return event.InternalServerError("Não foi possível carregar a atividade.", err)
	}

	browserNames := make(map[string]string)
	result := make([]activityResponse, 0, len(records))
	for _, record := range records {
		browserID := record.GetString("browser")
		browserName := browserNames[browserID]
		if browserID != "" && browserName == "" {
			if browser, findErr := event.App.FindRecordById(pb_migrations.BrowsersCollection, browserID); findErr == nil && browser.GetString("owner") == event.Auth.Id {
				browserName = browser.GetString("name")
				browserNames[browserID] = browserName
			}
		}
		result = append(result, activityResponse{
			ID:          record.Id,
			Event:       record.GetString("event"),
			Result:      record.GetString("result"),
			BrowserID:   browserID,
			BrowserName: browserName,
			CreatedAt:   record.GetDateTime("created").String(),
		})
	}
	return event.JSON(http.StatusOK, result)
}

func listBrowsers(event *core.RequestEvent) error {
	records, err := event.App.FindRecordsByFilter(
		pb_migrations.BrowsersCollection,
		"owner = {:owner} && state != 'deleting'",
		"-created",
		maxBrowsersPerUser,
		0,
		dbx.Params{"owner": event.Auth.Id},
	)
	if err != nil {
		return event.InternalServerError("Não foi possível listar os navegadores.", err)
	}

	result := make([]browserResponse, 0, len(records))
	for _, record := range records {
		result = append(result, mapBrowser(record))
	}
	return event.JSON(http.StatusOK, result)
}

func createBrowser(event *core.RequestEvent) error {
	var input struct {
		Name string `json:"name"`
	}
	if err := event.BindBody(&input); err != nil {
		return event.BadRequestError("Corpo da requisição inválido.", err)
	}
	name, err := normalizeBrowserName(input.Name)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}

	existing, err := event.App.FindRecordsByFilter(
		pb_migrations.BrowsersCollection,
		"owner = {:owner} && state != 'deleting'",
		"",
		maxBrowsersPerUser+1,
		0,
		dbx.Params{"owner": event.Auth.Id},
	)
	if err != nil {
		return event.InternalServerError("Não foi possível validar o limite de navegadores.", err)
	}
	if len(existing) >= maxBrowsersPerUser {
		return event.BadRequestError(fmt.Sprintf("O limite atual é de %d navegadores por conta.", maxBrowsersPerUser), nil)
	}

	collection, err := event.App.FindCollectionByNameOrId(pb_migrations.BrowsersCollection)
	if err != nil {
		return event.InternalServerError("Coleção de navegadores indisponível.", err)
	}
	record := core.NewRecord(collection)
	record.Set("owner", event.Auth.Id)
	record.Set("name", name)
	record.Set("state", "queued")
	record.Set("last_title", "Aguardando o Navego Agent")
	if err := event.App.Save(record); err != nil {
		return event.BadRequestError("Não foi possível criar o navegador.", err)
	}
	writeAudit(event.App, event.Auth.Id, record.Id, "browser.create", "success", map[string]any{"name": name})
	return event.JSON(http.StatusAccepted, mapBrowser(record))
}

func renameBrowser(event *core.RequestEvent) error {
	record, apiErr := ownedBrowser(event)
	if apiErr != nil {
		return apiErr
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := event.BindBody(&input); err != nil {
		return event.BadRequestError("Corpo da requisição inválido.", err)
	}
	name, err := normalizeBrowserName(input.Name)
	if err != nil {
		return event.BadRequestError(err.Error(), nil)
	}
	record.Set("name", name)
	if err := event.App.Save(record); err != nil {
		return event.BadRequestError("Não foi possível renomear o navegador.", err)
	}
	writeAudit(event.App, event.Auth.Id, record.Id, "browser.rename", "success", map[string]any{"name": name})
	return event.JSON(http.StatusOK, mapBrowser(record))
}

func powerBrowser(event *core.RequestEvent) error {
	record, apiErr := ownedBrowser(event)
	if apiErr != nil {
		return apiErr
	}
	var input struct {
		Running bool `json:"running"`
	}
	if err := event.BindBody(&input); err != nil {
		return event.BadRequestError("Corpo da requisição inválido.", err)
	}
	state := "stopping"
	operation := "browser.stop"
	if input.Running {
		state = "queued"
		operation = "browser.start"
	}
	record.Set("state", state)
	if err := event.App.Save(record); err != nil {
		return event.BadRequestError("Não foi possível alterar o navegador.", err)
	}
	writeAudit(event.App, event.Auth.Id, record.Id, operation, "success", nil)
	return event.JSON(http.StatusAccepted, mapBrowser(record))
}

func deleteBrowser(event *core.RequestEvent) error {
	record, apiErr := ownedBrowser(event)
	if apiErr != nil {
		return apiErr
	}
	record.Set("state", "deleting")
	if err := event.App.Save(record); err != nil {
		return event.BadRequestError("Não foi possível agendar a exclusão do navegador.", err)
	}
	writeAudit(event.App, event.Auth.Id, record.Id, "browser.delete", "success", nil)
	return event.JSON(http.StatusAccepted, map[string]string{"status": "deleting"})
}

func requireAgentToken(expected string) func(*core.RequestEvent) error {
	expectedBytes := []byte(expected)
	return func(event *core.RequestEvent) error {
		provided := strings.TrimSpace(strings.TrimPrefix(event.Request.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(expectedBytes) || subtle.ConstantTimeCompare([]byte(provided), expectedBytes) != 1 {
			return event.UnauthorizedError("Credencial do Navego Agent inválida.", nil)
		}
		return event.Next()
	}
}

func listAgentCommands(event *core.RequestEvent) error {
	agentID := strings.TrimSpace(event.Request.URL.Query().Get("agent_id"))
	if agentID == "" || len(agentID) > 100 {
		return event.BadRequestError("agent_id inválido.", nil)
	}

	records, err := event.App.FindRecordsByFilter(
		pb_migrations.BrowsersCollection,
		"(state = 'queued' || state = 'starting' || state = 'running' || state = 'stopping' || state = 'deleting') && (agent_id = '' || agent_id = {:agent})",
		"created",
		100,
		0,
		dbx.Params{"agent": agentID},
	)
	if err != nil {
		return event.InternalServerError("Não foi possível carregar os comandos do agente.", err)
	}

	result := make([]agentBrowser, 0, len(records))
	for _, record := range records {
		result = append(result, agentBrowser{
			ID:               record.Id,
			OwnerID:          record.GetString("owner"),
			Name:             record.GetString("name"),
			State:            record.GetString("state"),
			BrowserContainer: record.GetString("browser_container"),
			WorkerContainer:  record.GetString("worker_container"),
			ProfileVolume:    record.GetString("profile_volume"),
		})
	}
	return event.JSON(http.StatusOK, result)
}

func reportBrowserState(event *core.RequestEvent) error {
	record, err := event.App.FindRecordById(pb_migrations.BrowsersCollection, strings.TrimSpace(event.Request.PathValue("id")))
	if err != nil {
		return event.NotFoundError("Navegador não encontrado.", err)
	}

	var input struct {
		AgentID          string `json:"agent_id"`
		State            string `json:"state"`
		BrowserContainer string `json:"browser_container"`
		WorkerContainer  string `json:"worker_container"`
		ProfileVolume    string `json:"profile_volume"`
		WorkerEndpoint   string `json:"worker_endpoint"`
		ViewerEndpoint   string `json:"viewer_endpoint"`
		Title            string `json:"title"`
		URL              string `json:"url"`
		LastError        string `json:"last_error"`
	}
	if err := event.BindBody(&input); err != nil {
		return event.BadRequestError("Corpo da requisição inválido.", err)
	}
	input.AgentID = strings.TrimSpace(input.AgentID)
	if input.AgentID == "" || len(input.AgentID) > 100 {
		return event.BadRequestError("agent_id inválido.", nil)
	}
	assignedAgent := record.GetString("agent_id")
	if assignedAgent != "" && assignedAgent != input.AgentID {
		return apis.NewApiError(http.StatusConflict, "O navegador pertence a outro agente.", nil)
	}
	if !validAgentTransition(record.GetString("state"), input.State) {
		return apis.NewApiError(http.StatusConflict, "O estado desejado mudou durante a operação.", nil)
	}

	record.Set("agent_id", input.AgentID)
	record.Set("state", input.State)
	record.Set("browser_container", strings.TrimSpace(input.BrowserContainer))
	record.Set("worker_container", strings.TrimSpace(input.WorkerContainer))
	record.Set("profile_volume", strings.TrimSpace(input.ProfileVolume))
	record.Set("worker_endpoint", strings.TrimSpace(input.WorkerEndpoint))
	record.Set("viewer_endpoint", strings.TrimSpace(input.ViewerEndpoint))
	record.Set("last_error", truncate(strings.TrimSpace(input.LastError), 2000))
	if input.State == "running" {
		title := truncate(strings.TrimSpace(input.Title), 500)
		if title != "" {
			record.Set("last_title", title)
		} else if record.GetString("last_title") == "" || record.GetString("last_title") == "Aguardando o Navego Agent" {
			record.Set("last_title", "Chromium disponível")
		}
		if pageURL := truncate(strings.TrimSpace(input.URL), 2048); pageURL != "" {
			record.Set("last_url", pageURL)
		}
		record.Set("last_seen", time.Now().UTC())
	}
	if err := event.App.Save(record); err != nil {
		return event.BadRequestError("Não foi possível atualizar o navegador.", err)
	}
	return event.JSON(http.StatusOK, mapBrowser(record))
}

func confirmBrowserDeletion(event *core.RequestEvent) error {
	record, err := event.App.FindRecordById(pb_migrations.BrowsersCollection, strings.TrimSpace(event.Request.PathValue("id")))
	if err != nil {
		return event.NotFoundError("Navegador não encontrado.", err)
	}
	if record.GetString("state") != "deleting" {
		return apis.NewApiError(http.StatusConflict, "O navegador não está marcado para exclusão.", nil)
	}
	if err := event.App.Delete(record); err != nil {
		return event.InternalServerError("Não foi possível concluir a exclusão.", err)
	}
	return event.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func validAgentTransition(current, next string) bool {
	if next == "error" {
		return current != "deleting"
	}
	switch next {
	case "starting":
		return current == "queued" || current == "starting"
	case "running":
		return current == "queued" || current == "starting" || current == "running"
	case "stopped":
		return current == "stopping" || current == "stopped"
	default:
		return false
	}
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func ownedBrowser(event *core.RequestEvent) (*core.Record, error) {
	id := strings.TrimSpace(event.Request.PathValue("id"))
	record, err := event.App.FindRecordById(pb_migrations.BrowsersCollection, id)
	if err != nil || record.GetString("owner") != event.Auth.Id || record.GetString("state") == "deleting" {
		return nil, event.NotFoundError("Navegador não encontrado.", err)
	}
	return record, nil
}

func normalizeBrowserName(name string) (string, error) {
	name = strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	if len([]rune(name)) < 2 || len([]rune(name)) > 48 {
		return "", fmt.Errorf("o nome deve ter entre 2 e 48 caracteres")
	}
	return name, nil
}

func mapBrowser(record *core.Record) browserResponse {
	return browserResponse{
		ID:        record.Id,
		Name:      record.GetString("name"),
		State:     record.GetString("state"),
		Title:     record.GetString("last_title"),
		URL:       record.GetString("last_url"),
		UpdatedAt: record.GetDateTime("updated").String(),
	}
}

func writeAudit(app core.App, ownerID, browserID, eventName, result string, metadata map[string]any) {
	collection, err := app.FindCollectionByNameOrId(pb_migrations.AuditEventsCollection)
	if err != nil {
		return
	}
	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("browser", browserID)
	record.Set("event", eventName)
	record.Set("result", result)
	if metadata != nil {
		record.Set("metadata", metadata)
	}
	_ = app.Save(record)
}
