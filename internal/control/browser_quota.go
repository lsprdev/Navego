package control

import (
	"fmt"
	"net/http"

	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func reserveBrowser(app core.App, ownerID, name string, cfg Config) (*core.Record, bool, error) {
	var record *core.Record
	var isDefault bool
	// PocketBase serializes this write transaction. Count + insert + default
	// selection must be atomic, including under concurrent HTTP requests.
	err := app.RunInTransaction(func(tx core.App) error {
		owned, err := tx.CountRecords(pb_migrations.BrowsersCollection, dbx.HashExp{"owner": ownerID})
		if err != nil {
			return err
		}
		if owned >= int64(cfg.MaxBrowsersPerUser) {
			return apis.NewApiError(http.StatusConflict, fmt.Sprintf("Limite de %d navegadores por conta atingido. Exclusões pendentes ainda ocupam vaga.", cfg.MaxBrowsersPerUser), nil)
		}
		total, err := tx.CountRecords(pb_migrations.BrowsersCollection)
		if err != nil {
			return err
		}
		if total >= int64(cfg.MaxBrowsersTotal) {
			return apis.NewApiError(http.StatusConflict, "O servidor atingiu o limite de navegadores. Aguarde uma vaga ou contate o administrador.", nil)
		}
		collection, err := tx.FindCollectionByNameOrId(pb_migrations.BrowsersCollection)
		if err != nil {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("owner", ownerID)
		record.Set("name", name)
		record.Set("state", "queued")
		record.Set("last_title", "Aguardando o Navego Agent")
		if err := tx.Save(record); err != nil {
			return err
		}
		owner, err := tx.FindRecordById("users", ownerID)
		if err != nil {
			return err
		}
		isDefault = owner.GetString("default_browser") == ""
		if isDefault {
			owner.Set("default_browser", record.Id)
			return tx.Save(owner)
		}
		return nil
	})
	return record, isDefault, err
}
