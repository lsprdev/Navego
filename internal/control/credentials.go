package control

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/lsprdev/Navego/internal/credentials"
	"github.com/lsprdev/Navego/pb_migrations"
)

type credentialResponse struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Origin    string `json:"origin"`
	Username  string `json:"username"`
	UpdatedAt string `json:"updated_at"`
}

type credentialInput struct {
	Label    string `json:"label"`
	Origin   string `json:"origin"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func listCredentials(vault *credentialVault, vaultErr error) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		if apiErr := vaultAvailabilityError(vault, vaultErr); apiErr != nil {
			return apiErr
		}
		records, err := event.App.FindRecordsByFilter(
			pb_migrations.CredentialsCollection,
			"owner = {:owner}",
			"label",
			100,
			0,
			dbx.Params{"owner": event.Auth.Id},
		)
		if err != nil {
			return event.InternalServerError("Não foi possível carregar os acessos salvos.", err)
		}
		result := make([]credentialResponse, 0, len(records))
		for _, record := range records {
			item, err := mapCredential(vault, event.Auth.Id, record)
			if err != nil {
				return event.InternalServerError("Um acesso salvo não pôde ser decifrado.", err)
			}
			result = append(result, item)
		}
		return event.JSON(http.StatusOK, result)
	}
}

func createCredential(vault *credentialVault, vaultErr error) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		if apiErr := vaultAvailabilityError(vault, vaultErr); apiErr != nil {
			return apiErr
		}
		var input credentialInput
		if err := event.BindBody(&input); err != nil {
			return event.BadRequestError("Corpo da requisição inválido.", err)
		}
		label, origin, username, password, err := normalizeCredential(input, true)
		if err != nil {
			return event.BadRequestError(err.Error(), nil)
		}
		payload, err := vault.encrypt(event.Auth.Id, origin, credentialSecret{Username: username, Password: password})
		if err != nil {
			return event.InternalServerError("Não foi possível cifrar o acesso.", err)
		}

		collection, err := event.App.FindCollectionByNameOrId(pb_migrations.CredentialsCollection)
		if err != nil {
			return event.InternalServerError("Coleção de acessos indisponível.", err)
		}
		record := core.NewRecord(collection)
		record.Set("owner", event.Auth.Id)
		record.Set("label", label)
		record.Set("origin", origin)
		record.Set("encrypted_payload", payload)
		record.Set("key_version", credentialKeyVersion)
		if err := event.App.Save(record); err != nil {
			return event.BadRequestError("Não foi possível salvar o acesso. Verifique se já existe um login para esse site.", err)
		}
		writeAudit(event.App, event.Auth.Id, "", "credential.create", "success", map[string]any{"label": label})
		return event.JSON(http.StatusCreated, credentialResponse{
			ID: record.Id, Label: label, Origin: origin, Username: username,
			UpdatedAt: record.GetDateTime("updated").String(),
		})
	}
}

func updateCredential(vault *credentialVault, vaultErr error) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		if apiErr := vaultAvailabilityError(vault, vaultErr); apiErr != nil {
			return apiErr
		}
		record, apiErr := ownedCredential(event)
		if apiErr != nil {
			return apiErr
		}
		var input credentialInput
		if err := event.BindBody(&input); err != nil {
			return event.BadRequestError("Corpo da requisição inválido.", err)
		}
		label, origin, username, password, err := normalizeCredential(input, false)
		if err != nil {
			return event.BadRequestError(err.Error(), nil)
		}
		if password == "" {
			existing, err := vault.decrypt(event.Auth.Id, record.GetString("origin"), record.GetString("encrypted_payload"))
			if err != nil {
				return event.InternalServerError("O acesso salvo não pôde ser decifrado.", err)
			}
			password = existing.Password
		}
		payload, err := vault.encrypt(event.Auth.Id, origin, credentialSecret{Username: username, Password: password})
		if err != nil {
			return event.InternalServerError("Não foi possível cifrar o acesso.", err)
		}
		record.Set("label", label)
		record.Set("origin", origin)
		record.Set("encrypted_payload", payload)
		record.Set("key_version", credentialKeyVersion)
		if err := event.App.Save(record); err != nil {
			return event.BadRequestError("Não foi possível atualizar o acesso. Verifique se já existe um login para esse site.", err)
		}
		writeAudit(event.App, event.Auth.Id, "", "credential.update", "success", map[string]any{"label": label})
		return event.JSON(http.StatusOK, credentialResponse{
			ID: record.Id, Label: label, Origin: origin, Username: username,
			UpdatedAt: record.GetDateTime("updated").String(),
		})
	}
}

func deleteCredential(_ *credentialVault, _ error) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		record, apiErr := ownedCredential(event)
		if apiErr != nil {
			return apiErr
		}
		label := record.GetString("label")
		if err := event.App.Delete(record); err != nil {
			return event.InternalServerError("Não foi possível excluir o acesso.", err)
		}
		writeAudit(event.App, event.Auth.Id, "", "credential.delete", "success", map[string]any{"label": label})
		return event.JSON(http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func ownedCredential(event *core.RequestEvent) (*core.Record, error) {
	record, err := event.App.FindRecordById(
		pb_migrations.CredentialsCollection,
		strings.TrimSpace(event.Request.PathValue("id")),
	)
	if err != nil || record.GetString("owner") != event.Auth.Id {
		return nil, event.NotFoundError("Acesso salvo não encontrado.", err)
	}
	return record, nil
}

func mapCredential(vault *credentialVault, ownerID string, record *core.Record) (credentialResponse, error) {
	if record.GetInt("key_version") != credentialKeyVersion {
		return credentialResponse{}, fmt.Errorf("unsupported credential key version")
	}
	secret, err := vault.decrypt(ownerID, record.GetString("origin"), record.GetString("encrypted_payload"))
	if err != nil {
		return credentialResponse{}, err
	}
	return credentialResponse{
		ID:        record.Id,
		Label:     record.GetString("label"),
		Origin:    record.GetString("origin"),
		Username:  secret.Username,
		UpdatedAt: record.GetDateTime("updated").String(),
	}, nil
}

func normalizeCredential(input credentialInput, requirePassword bool) (string, string, string, string, error) {
	label := strings.Join(strings.Fields(strings.TrimSpace(input.Label)), " ")
	if length := len([]rune(label)); length < 2 || length > 80 {
		return "", "", "", "", fmt.Errorf("o nome deve ter entre 2 e 80 caracteres")
	}
	origin, err := credentials.CanonicalOrigin(input.Origin)
	if err != nil {
		return "", "", "", "", fmt.Errorf("site inválido: use somente a origem HTTPS, como https://exemplo.com")
	}
	username := strings.TrimSpace(input.Username)
	if length := len([]rune(username)); length < 1 || length > 500 {
		return "", "", "", "", fmt.Errorf("o usuário deve ter entre 1 e 500 caracteres")
	}
	password := input.Password
	if requirePassword && password == "" {
		return "", "", "", "", fmt.Errorf("informe a senha")
	}
	if len([]byte(password)) > 16<<10 {
		return "", "", "", "", fmt.Errorf("a senha excede o limite permitido")
	}
	return label, origin, username, password, nil
}

func vaultAvailabilityError(vault *credentialVault, vaultErr error) error {
	if vault == nil || vaultErr != nil {
		return apis.NewApiError(
			http.StatusServiceUnavailable,
			"O cofre de acessos ainda não foi configurado neste servidor.",
			nil,
		)
	}
	return nil
}
