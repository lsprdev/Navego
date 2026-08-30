package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/migrations"
)

// This migration turns the original one-browser OAuth draft into a user-scoped
// grant. A connected client can address any browser owned by the user, while
// default_browser remains only the fallback for calls without a selector.
func init() {
	migrations.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		browsers, err := app.FindCollectionByNameOrId(BrowsersCollection)
		if err != nil {
			return err
		}
		users.Fields.Add(&core.RelationField{
			Name:         "default_browser",
			CollectionId: browsers.Id,
			MaxSelect:    1,
		})
		if err := app.Save(users); err != nil {
			return err
		}

		grants, err := app.FindCollectionByNameOrId(OAuthGrantsCollection)
		if err != nil {
			return err
		}
		if browser, ok := grants.Fields.GetByName("browser").(*core.RelationField); ok {
			browser.Required = false
		}
		grants.Fields.Add(&core.TextField{Name: "resource", Required: true, Max: 2048, Hidden: true})
		if err := app.Save(grants); err != nil {
			return err
		}

		refresh, err := app.FindCollectionByNameOrId(OAuthRefreshCollection)
		if err != nil {
			return err
		}
		if browser, ok := refresh.Fields.GetByName("browser").(*core.RelationField); ok {
			browser.Required = false
		}
		refresh.Fields.Add(&core.TextField{Name: "resource", Required: true, Max: 2048, Hidden: true})
		if err := app.Save(refresh); err != nil {
			return err
		}

		clients := core.NewBaseCollection(OAuthClientsCollection)
		clients.Fields.Add(
			&core.TextField{Name: "client_id", Required: true, Max: 200, Hidden: true},
			&core.TextField{Name: "client_name", Required: true, Max: 200},
			&core.JSONField{Name: "redirect_uris", Required: true, MaxSize: 16 << 10, Hidden: true},
			&core.TextField{Name: "token_endpoint_auth_method", Required: true, Max: 50, Hidden: true},
			createdField(),
			updatedField(),
		)
		clients.AddIndex("idx_oauth_clients_client_id", true, "client_id", "")
		if err := app.Save(clients); err != nil {
			return err
		}

		access := core.NewBaseCollection(OAuthAccessCollection)
		access.Fields.Add(
			ownerField(users.Id),
			&core.TextField{Name: "client_id", Required: true, Max: 500},
			&core.TextField{Name: "token_hash", Required: true, Max: 500, Hidden: true},
			&core.TextField{Name: "scopes", Required: true, Max: 1000},
			&core.TextField{Name: "resource", Required: true, Max: 2048, Hidden: true},
			&core.DateField{Name: "expires_at", Required: true},
			&core.DateField{Name: "revoked_at"},
			createdField(),
			updatedField(),
		)
		access.AddIndex("idx_oauth_access_token_hash", true, "token_hash", "")
		if err := app.Save(access); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		if collection, err := app.FindCollectionByNameOrId(OAuthAccessCollection); err == nil {
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		if collection, err := app.FindCollectionByNameOrId(OAuthClientsCollection); err == nil {
			if err := app.Delete(collection); err != nil {
				return err
			}
		}

		if collection, err := app.FindCollectionByNameOrId(OAuthRefreshCollection); err == nil {
			collection.Fields.RemoveByName("resource")
			if browser, ok := collection.Fields.GetByName("browser").(*core.RelationField); ok {
				browser.Required = true
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		if collection, err := app.FindCollectionByNameOrId(OAuthGrantsCollection); err == nil {
			collection.Fields.RemoveByName("resource")
			if browser, ok := collection.Fields.GetByName("browser").(*core.RelationField); ok {
				browser.Required = true
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		if users, err := app.FindCollectionByNameOrId("users"); err == nil {
			users.Fields.RemoveByName("default_browser")
			return app.Save(users)
		}
		return nil
	})
}
