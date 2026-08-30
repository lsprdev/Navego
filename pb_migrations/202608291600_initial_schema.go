package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	BrowsersCollection     = "browsers"
	CredentialsCollection  = "saved_credentials"
	OAuthGrantsCollection  = "oauth_grants"
	OAuthRefreshCollection = "oauth_refresh_tokens"
	OAuthClientsCollection = "oauth_clients"
	OAuthAccessCollection  = "oauth_access_tokens"
	AuditEventsCollection  = "audit_events"
)

func init() {
	migrations.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		browsers := core.NewBaseCollection(BrowsersCollection)
		browsers.ListRule = types.Pointer("owner = @request.auth.id")
		browsers.ViewRule = types.Pointer("owner = @request.auth.id")
		browsers.Fields.Add(
			ownerField(users.Id),
			&core.TextField{Name: "name", Required: true, Max: 48, Presentable: true},
			&core.SelectField{
				Name: "state",
				Values: []string{
					"queued", "starting", "running", "stopping", "stopped", "deleting", "error",
				},
				Required: true,
			},
			&core.TextField{Name: "last_title", Max: 500},
			&core.TextField{Name: "last_url", Max: 2048},
			&core.TextField{Name: "last_error", Max: 2000},
			&core.DateField{Name: "last_seen"},
			&core.TextField{Name: "agent_id", Max: 100, Hidden: true},
			&core.TextField{Name: "browser_container", Max: 200, Hidden: true},
			&core.TextField{Name: "worker_container", Max: 200, Hidden: true},
			&core.TextField{Name: "profile_volume", Max: 200, Hidden: true},
			&core.TextField{Name: "worker_endpoint", Max: 500, Hidden: true},
			&core.TextField{Name: "viewer_endpoint", Max: 500, Hidden: true},
			createdField(),
			updatedField(),
		)
		browsers.AddIndex("idx_browsers_owner_name", true, "owner, name", "state != 'deleting'")
		browsers.AddIndex("idx_browsers_state", false, "state", "")
		if err := app.Save(browsers); err != nil {
			return err
		}

		credentials := core.NewBaseCollection(CredentialsCollection)
		credentials.ListRule = types.Pointer("owner = @request.auth.id")
		credentials.ViewRule = types.Pointer("owner = @request.auth.id")
		credentials.Fields.Add(
			ownerField(users.Id),
			&core.TextField{Name: "label", Required: true, Max: 80, Presentable: true},
			&core.URLField{Name: "origin", Required: true, OnlyDomains: []string{}},
			&core.TextField{Name: "encrypted_payload", Required: true, Max: 32 << 10, Hidden: true},
			&core.NumberField{Name: "key_version", Required: true, OnlyInt: true, Hidden: true},
			createdField(),
			updatedField(),
		)
		credentials.AddIndex("idx_saved_credentials_owner_origin", true, "owner, origin", "")
		if err := app.Save(credentials); err != nil {
			return err
		}

		oauthGrants := core.NewBaseCollection(OAuthGrantsCollection)
		oauthGrants.Fields.Add(
			ownerField(users.Id),
			&core.RelationField{Name: "browser", CollectionId: browsers.Id, Required: true, CascadeDelete: true, MaxSelect: 1},
			&core.TextField{Name: "client_id", Required: true, Max: 500},
			&core.TextField{Name: "scopes", Required: true, Max: 1000},
			&core.TextField{Name: "code_hash", Max: 500, Hidden: true},
			&core.TextField{Name: "code_challenge", Max: 500, Hidden: true},
			&core.TextField{Name: "redirect_uri", Max: 2048, Hidden: true},
			&core.DateField{Name: "expires_at", Required: true},
			&core.DateField{Name: "used_at"},
			createdField(),
			updatedField(),
		)
		oauthGrants.AddIndex("idx_oauth_grants_code_hash", true, "code_hash", "code_hash != ''")
		if err := app.Save(oauthGrants); err != nil {
			return err
		}

		oauthRefresh := core.NewBaseCollection(OAuthRefreshCollection)
		oauthRefresh.Fields.Add(
			ownerField(users.Id),
			&core.RelationField{Name: "browser", CollectionId: browsers.Id, Required: true, CascadeDelete: true, MaxSelect: 1},
			&core.TextField{Name: "client_id", Required: true, Max: 500},
			&core.TextField{Name: "token_hash", Required: true, Max: 500, Hidden: true},
			&core.TextField{Name: "scopes", Required: true, Max: 1000},
			&core.DateField{Name: "expires_at", Required: true},
			&core.DateField{Name: "revoked_at"},
			createdField(),
			updatedField(),
		)
		oauthRefresh.AddIndex("idx_oauth_refresh_token_hash", true, "token_hash", "")
		if err := app.Save(oauthRefresh); err != nil {
			return err
		}

		audit := core.NewBaseCollection(AuditEventsCollection)
		audit.ListRule = types.Pointer("owner = @request.auth.id")
		audit.ViewRule = types.Pointer("owner = @request.auth.id")
		audit.Fields.Add(
			ownerField(users.Id),
			&core.RelationField{Name: "browser", CollectionId: browsers.Id, CascadeDelete: false, MaxSelect: 1},
			&core.TextField{Name: "event", Required: true, Max: 100, Presentable: true},
			&core.SelectField{Name: "result", Required: true, Values: []string{"success", "denied", "error"}},
			&core.JSONField{Name: "metadata", MaxSize: 32 << 10},
			createdField(),
		)
		audit.AddIndex("idx_audit_owner_created", false, "owner, created", "")
		if err := app.Save(audit); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		for _, name := range []string{
			AuditEventsCollection,
			OAuthRefreshCollection,
			OAuthGrantsCollection,
			CredentialsCollection,
			BrowsersCollection,
		} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		return nil
	})
}

func ownerField(usersCollectionID string) *core.RelationField {
	return &core.RelationField{
		Name:          "owner",
		CollectionId:  usersCollectionID,
		CascadeDelete: true,
		MaxSelect:     1,
		Required:      true,
	}
}

func createdField() *core.AutodateField {
	return &core.AutodateField{Name: "created", OnCreate: true}
}

func updatedField() *core.AutodateField {
	return &core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true}
}
