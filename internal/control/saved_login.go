package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lsprdev/Navego/internal/credentials"
	"github.com/lsprdev/Navego/internal/httpserver"
	"github.com/lsprdev/Navego/internal/oauthresource"
	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/dbx"
)

var errSavedCredentialNotFound = errors.New("saved credential not found")

type savedLoginToolInput struct {
	Browser     string `json:"browser,omitempty"`
	UsernameRef string `json:"username_ref"`
	PasswordRef string `json:"password_ref"`
	SubmitRef   string `json:"submit_ref"`
}

type savedCredentialMaterial struct {
	Descriptor credentials.Descriptor
	Username   []byte
	Password   []byte
}

func (material *savedCredentialMaterial) Clear() {
	clear(material.Username)
	clear(material.Password)
	material.Username = nil
	material.Password = nil
}

func (s *multiBrowserMCP) addSavedLoginTool() {
	scopes := []string{oauthresource.ScopeRead, oauthresource.ScopeInteract, oauthresource.ScopeWrite}
	tool := &mcp.Tool{
		Name:  "browser_login_with_saved_access",
		Title: "Sign in with a saved Navego access",
		Description: "Use this first whenever the current Chromium shows a username/password login form. " +
			"Navego automatically selects the connected user's encrypted access by the page's exact HTTPS origin, submits it without exposing secrets, and returns the resulting page. " +
			"If no matching access exists, then request human login.",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"username_ref", "password_ref", "submit_ref"},
			"properties": map[string]any{
				"browser": map[string]any{
					"type": "string", "description": "Optional exact Navego browser name or ID. Omit only to use the user's default Chromium.",
				},
				"username_ref": map[string]any{"type": "string", "description": "Current snapshot ref for the username field."},
				"password_ref": map[string]any{"type": "string", "description": "Current snapshot ref for the password field."},
				"submit_ref":   map[string]any{"type": "string", "description": "Current snapshot ref for the sign-in button."},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, IdempotentHint: false, OpenWorldHint: boolRef(true), DestructiveHint: boolRef(false),
		},
		Meta: mcp.Meta{"securitySchemes": oauthSecurity(scopes)},
	}
	s.server.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, unauthorized := authorizeCentralTool(ctx, request, scopes, s.resourceMetaURL)
		if unauthorized != nil {
			return unauthorized, nil
		}
		var input savedLoginToolInput
		if request == nil || request.Params == nil || json.Unmarshal(request.Params.Arguments, &input) != nil {
			return toolError("Informe os refs atuais dos campos de usuário, senha e do botão de login."), nil
		}
		result, browserID := s.executeSavedLogin(ctx, info.UserID, input)
		status := "success"
		if result.IsError {
			status = "error"
		}
		writeAudit(s.app, info.UserID, browserID, "mcp.browser_login_with_saved_access", status, nil)
		return result, nil
	})
}

func (s *multiBrowserMCP) executeSavedLogin(ctx context.Context, ownerID string, input savedLoginToolInput) (*mcp.CallToolResult, string) {
	input.UsernameRef = strings.TrimSpace(input.UsernameRef)
	input.PasswordRef = strings.TrimSpace(input.PasswordRef)
	input.SubmitRef = strings.TrimSpace(input.SubmitRef)
	if input.UsernameRef == "" || input.PasswordRef == "" || input.SubmitRef == "" {
		return toolError("Os refs atuais dos campos de usuário, senha e do botão de login são obrigatórios."), ""
	}
	record, err := s.resolveBrowser(ownerID, input.Browser, true)
	if err != nil {
		return toolError(err.Error()), ""
	}
	browserID := record.Id
	endpoint, err := validatedWorkerEndpoint(record.GetString("worker_endpoint"))
	if err != nil {
		return toolError("O worker deste Chromium ainda não está disponível."), browserID
	}

	describeInput := httpserver.InternalSavedLoginDescribeRequest{
		UsernameRef: input.UsernameRef,
		PasswordRef: input.PasswordRef,
		SubmitRef:   input.SubmitRef,
	}
	var described httpserver.InternalSavedLoginDescribeResponse
	if err := s.postWorkerSavedLogin(ctx, endpoint+"/internal/saved-login/describe", describeInput, &described); err != nil {
		return toolError("Não foi possível validar o formulário de login atual: " + err.Error()), browserID
	}
	currentOrigin, err := credentials.OriginFromURL(described.Target.RawURL)
	if err != nil || currentOrigin != described.Target.Origin {
		return toolError("O formulário de login não possui uma origem HTTPS válida e consistente."), browserID
	}

	material, err := s.savedCredential(ownerID, currentOrigin)
	if err != nil {
		if errors.Is(err, errSavedCredentialNotFound) {
			return toolError(fmt.Sprintf("Nenhum acesso salvo corresponde à origem exata %s. Use browser_request_human_login agora.", currentOrigin)), browserID
		}
		return toolError("O cofre de acessos não pôde ser aberto: " + err.Error()), browserID
	}
	defer material.Clear()

	commitInput := httpserver.InternalSavedLoginCommitRequest{
		Target:   described.Target,
		Username: material.Username,
		Password: material.Password,
	}
	var committed httpserver.InternalSavedLoginCommitResponse
	if err := s.postWorkerSavedLogin(ctx, endpoint+"/internal/saved-login/commit", commitInput, &committed); err != nil {
		return toolError("O formulário mudou ou recusou o preenchimento protegido: " + err.Error()), browserID
	}

	message := fmt.Sprintf(
		"Acesso salvo %q usado com segurança no Chromium %s para %s. A credencial não foi incluída na chamada do ChatGPT nem no resultado. Página atual: %s (%s).",
		material.Descriptor.Label,
		record.GetString("name"),
		material.Descriptor.Origin,
		committed.Snapshot.Title,
		committed.Snapshot.URL,
	)
	return jsonToolResult(message, map[string]any{
		"status":   "signed_in",
		"browser":  map[string]any{"id": browserID, "name": record.GetString("name")},
		"account":  map[string]any{"label": material.Descriptor.Label, "origin": material.Descriptor.Origin},
		"snapshot": committed.Snapshot,
	}), browserID
}

func (s *multiBrowserMCP) savedCredential(ownerID, origin string) (savedCredentialMaterial, error) {
	if s.vault == nil || s.vaultErr != nil {
		return savedCredentialMaterial{}, errors.New("cofre indisponível")
	}
	records, err := s.app.FindRecordsByFilter(
		pb_migrations.CredentialsCollection,
		"owner = {:owner} && origin = {:origin}",
		"",
		1,
		0,
		dbx.Params{"owner": strings.TrimSpace(ownerID), "origin": strings.TrimSpace(origin)},
	)
	if err != nil {
		return savedCredentialMaterial{}, err
	}
	if len(records) == 0 {
		return savedCredentialMaterial{}, errSavedCredentialNotFound
	}
	record := records[0]
	if record.GetInt("key_version") != credentialKeyVersion {
		return savedCredentialMaterial{}, errors.New("versão de chave não suportada")
	}
	secret, err := s.vault.decrypt(ownerID, record.GetString("origin"), record.GetString("encrypted_payload"))
	if err != nil {
		return savedCredentialMaterial{}, err
	}
	return savedCredentialMaterial{
		Descriptor: credentials.Descriptor{ID: record.Id, Label: record.GetString("label"), Origin: record.GetString("origin")},
		Username:   []byte(secret.Username),
		Password:   []byte(secret.Password),
	}, nil
}

func (s *multiBrowserMCP) postWorkerSavedLogin(ctx context.Context, endpoint string, input, output any) error {
	key := strings.TrimSpace(s.cfg.WorkerAPIKey)
	if key == "" {
		return errors.New("broker interno não configurado")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return errors.New("não foi possível preparar a solicitação interna")
	}
	defer clear(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("não foi possível preparar a solicitação interna")
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := s.cfg.InternalHTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("worker indisponível")
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&failure); err != nil || strings.TrimSpace(failure.Error) == "" {
			return fmt.Errorf("worker respondeu %s", response.Status)
		}
		return errors.New(truncate(strings.TrimSpace(failure.Error), 300))
	}
	if err := decoder.Decode(output); err != nil {
		return errors.New("resposta interna inválida")
	}
	return nil
}
